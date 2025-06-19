package config

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/cloudbees-io/configure-eks-credentials/internal/core"
	"github.com/google/uuid"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// Config holds the authentication request configuration
type Config struct {
	// Name The name of the cluster for which to create a kubeconfig entry
	Name string
	// Region Override the default region and connect to a cluster in a different region
	Region string
	// RoleToAssume To assume a role for cluster authentication, specify an IAM role ARN with this option
	RoleToAssume string `mapstructure:"role-to-assume"`
	// RoleSessionName Session name to pass when assuming the IAM Role
	RoleSessionName string `mapstructure:"role-session-name"`
	// RoleExternalId External ID to pass when assuming the IAM Role
	RoleExternalId string `mapstructure:"role-external-id"`
	// ForwardSessionName Enable mapping a federated sessions caller-specified-role-name attribute onto newly assumed sessions
	ForwardSessionName bool `mapstructure:"forward-session-name"`
	// Alias The alias for the cluster context name
	Alias string
	// UserAlias The alias for the generated username
	UserAlias string `mapstructure:"user-alias"`
}

const HELPER_BINARY = "aws-iam-authenticator"

func (c *Config) Authenticate(ctx context.Context) error {
	core.Debug("name=%s", c.Name)
	if c.Name == "" {
		return fmt.Errorf("name must be specified")
	}

	if c.ForwardSessionName && c.RoleSessionName != "" {
		return fmt.Errorf("Error: cannot specify both forward-session-name and role-session-name parameter\n")
	}

	homePath := os.Getenv("HOME")
	kubePath := filepath.Join(homePath, ".kube")
	if err := os.MkdirAll(kubePath, os.ModePerm); err != nil {
		return err
	}
	configPath := filepath.Join(kubePath, "config")

	core.Debug("Loading existing config from %s", configPath)

	kubeconfig, err := clientcmd.LoadFromFile(configPath)
	if errors.Is(err, fs.ErrNotExist) {
		kubeconfig = &api.Config{}
		kubeconfig.Clusters = make(map[string]*api.Cluster)
		kubeconfig.Contexts = make(map[string]*api.Context)
		kubeconfig.AuthInfos = make(map[string]*api.AuthInfo)
	} else if err != nil {
		return err
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("could not load AWS credentials, have you run configure-aws-credentials first? %v", err)
	}

	if c.Region != "" {
		core.Debug("Overriding current region %s with %s", cfg.Region, c.Region)

		cfg.Region = c.Region
	}

	core.Debug("Describing cluster %s", c.Name)

	req := &eks.DescribeClusterInput{
		Name: &c.Name,
	}

	client := eks.NewFromConfig(cfg)
	cluster, err := client.DescribeCluster(ctx, req)
	if err != nil {
		return fmt.Errorf("could not fetch cluster details: %v", err)
	}

	clusterArn := *cluster.Cluster.Arn

	certData, err := base64.StdEncoding.DecodeString(*cluster.Cluster.CertificateAuthority.Data)
	if err != nil {
		return err
	}

	kubeconfig.Clusters[clusterArn] = &api.Cluster{
		Server:                   *cluster.Cluster.Endpoint,
		CertificateAuthorityData: certData,
	}

	if c.Alias == "" {
		c.Alias = clusterArn
	}
	if c.UserAlias == "" {
		c.UserAlias = clusterArn
	}

	kubeconfig.Contexts[c.Alias] = &api.Context{
		Cluster:  clusterArn,
		AuthInfo: c.UserAlias,
	}

	kubeconfig.CurrentContext = c.Alias

	args := []string{
		"token",
		"--cluster-id",
		c.Name,
	}

	if c.RoleToAssume != "" {
		client := sts.NewFromConfig(cfg)
		if !strings.HasPrefix(c.RoleToAssume, "arn:aws") {
			// Supports only 'aws' partition. Customers in other partitions ('aws-cn') will need to provide full ARN
			req := &sts.GetCallerIdentityInput{}
			if rsp, err := client.GetCallerIdentity(ctx, req); err != nil {
				return err
			} else {
				c.RoleToAssume = fmt.Sprintf("arn:aws:iam::%s:role/%s", *rsp.Account, c.RoleToAssume)
			}
		}

		core.Debug("role-to-assume=%s", c.RoleToAssume)
		args = append(args, "--role", c.RoleToAssume)

		core.Debug("role-external-id=%s", c.RoleExternalId)
		if c.RoleSessionName != "" {
			args = append(args, "--external-id", c.RoleExternalId)
		}

		core.Debug("forward-session-name=%v", c.ForwardSessionName)
		core.Debug("role-session-name=%s", c.RoleSessionName)
		if c.ForwardSessionName {
			args = append(args, "--forward-session-name")
		} else if c.RoleSessionName != "" {
			args = append(args, "--session-name", c.RoleSessionName)
		} else {
			args = append(args, "--session-name", "CloudBeesAutomations")
		}
	} else {
		core.Debug("role-to-assume=%s", c.RoleToAssume)
	}

	helperPath, err := exec.LookPath(HELPER_BINARY)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "WARNING: Cannot find helper binary %s on the path. Authentication will only work in containers that have this binary on the PATH\n", HELPER_BINARY)
		helperPath = HELPER_BINARY
	} else {
		newUUID, _ := uuid.NewUUID()
		targetPath := filepath.Join(kubePath, fmt.Sprintf("aws-iam-authenticator-%v", newUUID))
		if err := copyFileHelper(targetPath, helperPath); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "WARNING: Cannot copy helper binary %s to $HOME/.kube. Authentication will only work in containers that have this binary on the PATH: %v\n", HELPER_BINARY, err)
		} else {
			helperPath = targetPath
		}
	}

	kubeconfig.AuthInfos[c.UserAlias] = &api.AuthInfo{
		Exec: &api.ExecConfig{
			Command:         helperPath,
			Args:            args,
			APIVersion:      "client.authentication.k8s.io/v1beta1",
			Env:             []api.ExecEnvVar{},
			InteractiveMode: api.NeverExecInteractiveMode,
		},
	}

	core.Debug("Writing %s", configPath)
	return clientcmd.WriteToFile(*kubeconfig, configPath)
}

func copyFileHelper(dst string, src string) (err error) {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func(f *os.File) {
		err2 := f.Close()
		if err2 != nil && err == nil {
			err = err2
		}
	}(s)

	// Create the destination file with default permission
	d, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0555)
	if err != nil {
		return err
	}
	defer func(f *os.File) {
		err2 := f.Close()
		if err2 != nil && err == nil {
			err = err2
		}
	}(d)

	_, err = io.Copy(d, s)
	return err
}
