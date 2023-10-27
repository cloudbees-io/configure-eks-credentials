package cmd

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/cloudbees-io/configure-eks-credentials/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cmd = &cobra.Command{
		Use:          "configure-eks-credentials",
		Short:        "Configures credentials for accessing EKS",
		Long:         "Configures credentials for accessing EKS",
		SilenceUsage: true,
		RunE:         doConfigure,
	}
)

func Execute() error {
	return cmd.Execute()
}

func init() {
	viper.AutomaticEnv()

	viper.SetEnvPrefix("INPUT")

	replacer := strings.NewReplacer("-", "_")
	viper.SetEnvKeyReplacer(replacer)

	inputString("name", "", "The name of the cluster for which to create a kubeconfig entry")

	inputString("role-to-assume", "", "To assume a role for cluster authentication, specify an IAM role ARN with this option")

	inputString("role-session-name", "", "Session name to pass when assuming the IAM Role via `role-to-assume`")

	inputString("role-external-id", "", "External ID to pass when assuming the IAM Role via `role-to-assume`")

	inputBool("forward-session-name", false, "Enable mapping a federated sessions caller-specified-role-name attribute onto newly assumed sessions.")

	inputString("alias", "", "Alias for the cluster context name")

	inputString("user-alias", "", "Alias for the generated user name")
}

func inputString(name string, value string, usage string) {
	cmd.Flags().String(name, value, usage)
	_ = viper.BindPFlag(name, cmd.Flags().Lookup(name))
}

func inputBool(name string, value bool, usage string) {
	cmd.Flags().Bool(name, value, usage)
	_ = viper.BindPFlag(name, cmd.Flags().Lookup(name))
}

func cliContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cancel() // exit gracefully
		<-c
		os.Exit(1) // exit immediately on 2nd signal
	}()
	return ctx
}

func doConfigure(command *cobra.Command, args []string) error {
	ctx := cliContext()

	var cfg config.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return err
	}

	return cfg.Authenticate(ctx)
}
