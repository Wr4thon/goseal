package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v2"
)

func main() {
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name: "config",
				Aliases: []string{
					"c",
				},
			},
		},
		Name:    "goseal",
		Usage:   "Used to automatically generate kubernetes secret files (and optionally seal them)",
		Version: "v0.2.0",
		Commands: []*cli.Command{
			{
				Name:        "yaml",
				HelpName:    "yaml",
				Description: "creates a sealed secret from yaml input key-value pairs",
				Usage:       "Create a secret file with key-value pairs as in the yaml file",
				Aliases:     []string{"y"},
				Flags:       getStandardFlags(),
				Action:      Yaml,
			},
			{
				Name:        "file",
				HelpName:    "file",
				Description: "creates a (sealed) kubernetes secret with a file as secret value",
				Usage:       "Create a secret with a file as secret value.",
				Action:      File,
				Flags: append(getStandardFlags(), &cli.StringFlag{
					Name:     "key",
					Usage:    "the secret key, under which the file can be accessed",
					Aliases:  []string{"k"},
					Required: true,
				}),
			},
			{
				Name:        "config",
				HelpName:    "config",
				Description: "read, write or edit the configuration",
				Usage:       "read, write or edit the configuration",
				Aliases: []string{
					"c",
				},
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name: "global",
						Aliases: []string{
							"g",
						},
						Value: false,
					},
				},
				Subcommands: []*cli.Command{
					{
						Name:   "print",
						Action: PrintConfig,
					},
					// {
					// 	Name:   "add",
					// 	Action: AddConfig,
					// },
				},
			},
		},
	}

	app.EnableBashCompletion = true

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func getStandardFlags() []cli.Flag {
	return []cli.Flag{
		cli.BashCompletionFlag,
		cli.HelpFlag,
		&cli.StringFlag{
			Name:     "namespace",
			Usage:    "the namespace of the secret",
			Required: true,
			Aliases:  []string{"n"},
		},
		&cli.StringFlag{
			Name:     "file",
			Usage:    "the input file in yaml format",
			Required: true,
			Aliases:  []string{"f"},
		},
		&cli.StringFlag{
			Name:     "secret-name",
			Usage:    "the secret name",
			Required: true,
			Aliases:  []string{"s"},
		},
		&cli.StringFlag{
			Name:    "cert",
			Usage:   "if set, will run kubeseal with given cert",
			Aliases: []string{"c"},
		},
		&cli.StringFlag{
			Name:    "out",
			Usage:   "destination file",
			Aliases: []string{"o"},
		},
	}
}

// ErrEmptyFile is returned if the provided file has no content.
var ErrEmptyFile = errors.New("file content is empty")

// Yaml is a cli command
func Yaml(c *cli.Context) error {
	filePath := c.String("file")
	namespace := c.String("namespace")
	secretName := c.String("secret-name")
	certPath := c.String("cert")
	outFilenamePath := c.String("out")

	if err := handleConfig(c, &outFilenamePath, &certPath); err != nil {
		return err
	}

	file, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	if len(file) == 0 {
		return ErrEmptyFile
	}

	var secrets map[string]string

	if err := yaml.Unmarshal(file, &secrets); err != nil {
		return err
	}

	if err := ensureDirForFile(outFilenamePath); err != nil {
		return err
	}

	if certPath != "" {
		return sealSecret(secrets, secretName, namespace, certPath, outFilenamePath)
	}

	return createSecret(secrets, secretName, namespace, outFilenamePath)
}

func handleConfig(c *cli.Context, outPath, certPath *string) error {
	if c.IsSet("config") {
		configName := c.String("config")
		cfg, err := getStageConfigurationByName(c, configName)
		if err != nil {
			return err
		}

		// TODO path join
		*outPath = cfg.BasePath + "/" + *outPath
		*certPath = cfg.Cert
	}

	return nil
}

// File is a cli command
func File(c *cli.Context) error {
	filePath := c.String("file")
	secretKey := c.String("key")
	namespace := c.String("namespace")
	secretName := c.String("secret-name")
	certPath := c.String("cert")
	outFilenamePath := c.String("out")

	if err := handleConfig(c, &outFilenamePath, &certPath); err != nil {
		return err
	}

	file, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	if len(file) == 0 {
		return ErrEmptyFile
	}

	secrets := map[string]string{secretKey: string(file)}
	if err := ensureDirForFile(outFilenamePath); err != nil {
		return err
	}

	if certPath != "" {
		return sealSecret(secrets, secretName, namespace, certPath, outFilenamePath)
	}

	return createSecret(secrets, secretName, namespace, outFilenamePath)
}

// runs the kubectl create secret command and prints the output to stdout.
func createSecret(secrets map[string]string, secretName, namespace, outPath string) error {
	kubectlCreateSecret := getCreateSecretFileCmd(secrets, secretName, namespace)

	var stdout bytes.Buffer
	kubectlCreateSecret.Stdout = &stdout

	if err := runCommand(kubectlCreateSecret); err != nil {
		return err
	}

	if outPath == "" {
		fmt.Println(stdout.String())
		return nil
	}

	return os.WriteFile(outPath, stdout.Bytes(), os.ModePerm)
}

// runs the kubectl create secret command, pipes the output to the kubeseal command and prints the output to stdout.
func sealSecret(secrets map[string]string, secretName, namespace, certPath, outPath string) error {
	kubectlCreateSecret := getCreateSecretFileCmd(secrets, secretName, namespace)
	kubeseal := exec.Command("kubeseal", "--format", "yaml", "--cert", certPath)

	var (
		err            error
		stdout, stderr bytes.Buffer
	)

	kubeseal.Stdout = &stdout
	kubeseal.Stderr = &stderr

	// Get stdout of first command and attach it to stdin of second command.
	kubeseal.Stdin, err = kubectlCreateSecret.StdoutPipe()
	if err != nil {
		return err
	}

	if err := kubeseal.Start(); err != nil {
		return err
	}

	if err = runCommand(kubectlCreateSecret); err != nil {
		return err
	}

	if err = kubeseal.Wait(); err != nil {
		return errors.New(getErrText(err, kubeseal.Args, stderr.String()))
	}

	if outPath == "" {
		fmt.Println(stdout.String())
		return nil
	}

	return os.WriteFile(outPath, stdout.Bytes(), os.ModePerm)
}

// retrieve a printable error text from cmd errors
func getErrText(err error, cmdArgs []string, stdErr string) string {
	text := fmt.Sprintf(
		"command '%s' failed: %s",
		strings.Join(cmdArgs, " "),
		err.Error(),
	)

	errText := strings.TrimSpace(stdErr)
	if len(errText) > 0 {
		text += "\n" + errText
	}

	return text
}

func runCommand(cmd *exec.Cmd) error {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return errors.New(getErrText(err, cmd.Args, stderr.String()))
	}

	return nil
}

// creates
func getCreateSecretFileCmd(secrets map[string]string, secretName, namespace string) *exec.Cmd {
	args := []string{
		"create",
		"secret",
		"generic",
		secretName,
		"-n",
		namespace,
		"--dry-run",
		"-o",
		"yaml",
	}

	for k, v := range secrets {
		args = append(args, fmt.Sprintf("--from-literal=%s=%s", k, v))
	}

	return exec.Command("kubectl", args...)
}
