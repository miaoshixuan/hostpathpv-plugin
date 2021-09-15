/*Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kubectl_plugin

import (
	"ShixuanMiao/hostpathpv-plugin/pkg/hostpathclient"
	"context"
	"fmt"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	k8s "k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"strconv"
)

// GetOptions contains the input to the get command.
type GetOptions struct {
	genericclioptions.IOStreams
	client       *k8s.Clientset
	resource     string
	resourceName string
	filter       bool
}

var (
	support_type = []string{"node", "nodes", "pv", "pvs"}

	hostpathpv_valid_resources = `Valid resource types include:

    * node
    * pv
    `

	applyLong = templates.LongDesc(i18n.T(`
		Display one or many resources diskquota infomation.

		` + hostpathpv_valid_resources))

	applyExample = templates.Examples(i18n.T(`
		# List all node quota info in ps output format.
		kubectl hostpathpv get nodes

		# List a node quota info with specified NAME in ps output format.
		kubectl hostpathpv get node nodename

		# List all pvs quota info in ps output format.
		kubectl hostpathpv get pvs

		# List a pv quota info with specified NAME in ps output format.
		kubectl hostpathpv get pv pvname`))
)

// NewGetOptions returns a GetOptions with default chunk size 500.
func NewGetOptions(streams genericclioptions.IOStreams) *GetOptions {
	return &GetOptions{
		IOStreams: streams,
	}
}

func NewCmdGet(ctx context.Context, f cmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	o := NewGetOptions(streams)

	cmd := &cobra.Command{
		Use:     "get (TYPE/NAME ...) [flags]",
		Short:   i18n.T("Display node pod or pv quota information"),
		Long:    applyLong,
		Example: applyExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Validate(cmd, args))
			cmdutil.CheckErr(o.Complete(f, cmd, args))
			cmdutil.CheckErr(o.Run(ctx))
		},
	}
	cmd.Flags().Bool("filter", true, "Ignored no disk quota resource")
	return cmd
}

func (o *GetOptions) Validate(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return usageError(cmd, "Required resource not specified.")
	}

	typeSupported := false
	for _, t := range support_type {
		if t == args[0] {
			typeSupported = true
		}
	}
	if !typeSupported {
		return usageError(cmd, fmt.Sprintf("Resource type %s not support.", args[0]))
	}

	if len(args) > 2 {
		return usageError(cmd, "Only one or two argument can be passed.")
	}
	return nil
}

func (o *GetOptions) Complete(f cmdutil.Factory, cmd *cobra.Command, args []string) error {
	b, err := cmd.Flags().GetBool("filter")
	if err != nil {
		return fmt.Errorf("error accessing flag filter for command %s: %v", cmd.Name(), err)
	}
	o.filter = b

	client, err := f.KubernetesClientSet()
	if err != nil || client == nil {
		return noClientError(err)
	}
	o.client = client
	o.resource = args[0]
	if len(args) == 2 {
		o.resourceName = args[1]
	}
	return nil
}

func (o *GetOptions) Run(ctx context.Context) error {
	switch {
	case o.resource == "nodes" || o.resource == "node":
		return getNodes(ctx, o.client, o.resourceName, o.filter)
	case o.resource == "pvs" || o.resource == "pv":
		return getPVs(ctx, o.client, o.resourceName)
	}
	return nil
}

func getNodes(ctx context.Context, client *k8s.Clientset, nodeName string, filter bool) error {
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	if len(nodes.Items) == 0 {
		return nil
	}
	displayer := NewDisplayer("   ", "", "Name", "Status", "DiskNum", "Capacity", "Quota", "Used", "Persistent", "Ephemeral")
	var allCapacity, allQuota, allUsed, allPersistent, allEphemeral int64
	for _, node := range nodes.Items {
		if nodeName != "" && nodeName != node.Name {
			continue
		}
		hpNode := hostpathclient.ToHostPathNode(&node)
		quotaInfo := hpNode.GetDiskUsage()
		if filter && (!hpNode.IsHostPathNode() || len(quotaInfo.DiskStatus) == 0) {
			continue
		}
		displayer.AddLine(hpNode.GetName(), hpNode.GetReady(), fmt.Sprintf("(%d/%d)", len(quotaInfo.DiskStatus)-len(hpNode.GetDisabledDisk()), len(quotaInfo.DiskStatus)),
			convertIntToString(quotaInfo.Capacity),
			convertIntToString(quotaInfo.QuotaSize)+getPercentStr(quotaInfo.QuotaSize, quotaInfo.Capacity),
			convertIntToString(quotaInfo.Used)+getPercentStr(quotaInfo.Used, quotaInfo.Capacity),
			convertIntToString(quotaInfo.QuotaSize)+getPercentStr(quotaInfo.QuotaSize, quotaInfo.Capacity),
			convertIntToString(quotaInfo.QuotaSize)+getPercentStr(quotaInfo.QuotaSize, quotaInfo.Capacity))
		allCapacity += quotaInfo.Capacity
		allQuota += quotaInfo.QuotaSize
		allUsed += quotaInfo.Used
		allPersistent += quotaInfo.QuotaSize
		allEphemeral += quotaInfo.QuotaSize
	}
	displayer.Print(true)
	fmt.Printf("AllCapacity: %s, AllQuota: %s, AllUsed: %s, AllPersistent: %s, AllEphemeral: %s\n\n",
		convertIntToString(allCapacity), convertIntToString(allQuota)+getPercentStr(allQuota, allCapacity),
		convertIntToString(allUsed)+getPercentStr(allUsed, allCapacity),
		convertIntToString(allPersistent)+getPercentStr(allPersistent, allCapacity),
		convertIntToString(allEphemeral)+getPercentStr(allEphemeral, allCapacity))
	return nil
}

func getPVs(ctx context.Context, client *k8s.Clientset, pvName string) error {
	pvs, err := client.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	displayer := NewDisplayer("   ", "", "Name", "Status", "BindNode", "Capacity", "Ephemeral", "Quota", "Used")
	for _, pv := range pvs.Items {
		if pvName != "" && pvName != pv.Name {
			continue
		}
		hpv := hostpathclient.ToHostPathVolume(&pv)
		if !hpv.IsHostPath() {
			continue
		}
		displayer.AddLine(
			pv.GetName(),
			string(pv.Status.Phase),
			hpv.GetBindNodeName(),
			convertIntToString(hpv.GetCapacity()),
			strconv.FormatBool(hpv.IsEphemeral()),
			convertIntToString(int64(hpv.GetHardQuota())),
			convertIntToString(int64(hpv.GetUsage())))
	}
	displayer.Print(true)
	return nil
}
