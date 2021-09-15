package kubectl_plugin

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func NewCmdHostPathPv(ctx context.Context, streams genericclioptions.IOStreams) *cobra.Command {

	cmds := &cobra.Command{
		Use:   "hostpathpv",
		Short: "hostpathpv controls the Kubernetes cluster hostpathpv manager",
		Long:  "hostpathpv controls the Kubernetes cluster hostpathpv manager.",
		Run:   runHelp,
	}

	flags := cmds.PersistentFlags()
	kubeConfigFlags := genericclioptions.NewConfigFlags(true)
	kubeConfigFlags.AddFlags(flags)
	matchVersionKubeConfigFlags := cmdutil.NewMatchVersionFlags(kubeConfigFlags)
	matchVersionKubeConfigFlags.AddFlags(flags)

	f := cmdutil.NewFactory(matchVersionKubeConfigFlags)

	cmds.AddCommand(NewCmdGet(ctx, f, streams))
	return cmds
}

func runHelp(cmd *cobra.Command, args []string) {
	cmd.Help()
}

func usageError(cmd *cobra.Command, format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s\nSee '%s -h' for help and examples.", msg, cmd.CommandPath())
}

func noClientError(err error) error {
	return fmt.Errorf("faild get init client,please check kube config file. %v", err)
}
