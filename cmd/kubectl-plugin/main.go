package main

import (
	cmd "ShixuanMiao/hostpathpv-plugin/pkg/kubectl-plugin"
	"context"
	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"os"
)

func main() {
	flags := pflag.NewFlagSet("kubectl-hostpathpv", pflag.ExitOnError)
	pflag.CommandLine = flags

	root := cmd.NewCmdHostPathPv(context.Background(), genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
