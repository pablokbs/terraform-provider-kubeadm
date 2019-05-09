package provisioner

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"path"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform/helper/schema"

	"github.com/inercia/terraform-provider-kubeadm/internal/ssh"
	"github.com/inercia/terraform-provider-kubeadm/pkg/common"
)

var (
	errNoInitConfigFound = errors.New("no init configuration obtained")
)

// doKubeadmInit performs a `kubeadm init` in the remote host
func doKubeadmInit(d *schema.ResourceData, configFile []byte) ssh.ApplyFunc {
	kubeadmConfigFile := path.Join(common.DefKubeadmInitConfPath)
	extraArgs := ""
	extraArgs += " " + getKubeadmIgnoredChecksArg(d)
	extraArgs += " " + getKubeadmNodenameArg(d)

	return ssh.ApplyComposed(
		ssh.ApplyIf(
			ssh.CheckFileExists(kubeadmConfigFile),
			ssh.DoExec("kubeadm reset --force")),
		ssh.DoUploadFile(bytes.NewReader(configFile), kubeadmConfigFile),
		ssh.DoExec(fmt.Sprintf("kubeadm init --config=%s %s", kubeadmConfigFile, extraArgs)),
	)
}

// doDownloadKubeconfig downloads a kubeconfig from the remote master
func doDownloadKubeconfig(d *schema.ResourceData) ssh.ApplyFunc {
	kubeconfig := getKubeconfig(d)
	if kubeconfig == "" {
		log.Printf("[DEBUG] [KUBEADM] no config_path specified: will not download kubeconfig")
		return ssh.EmptyAction()
	}
	return ssh.DoDownloadFile(common.DefAdminKubeconfig, kubeconfig)
}

// doLoadCNI loads the CNI driver
func doLoadCNI(d *schema.ResourceData) ssh.ApplyFunc {
	manifest := ""
	if cniPluginManifestOpt, ok := d.GetOk("config.cni_plugin_manifest"); ok {
		cniPluginManifest := strings.TrimSpace(cniPluginManifestOpt.(string))
		if len(cniPluginManifest) > 0 {
			manifest = cniPluginManifest
		}
	} else {
		if cniPluginOpt, ok := d.GetOk("config.cni_plugin"); ok {
			cniPlugin := strings.TrimSpace(strings.ToLower(cniPluginOpt.(string)))
			if len(cniPlugin) > 0 {
				log.Printf("[DEBUG] [KUBEADM] verifying CNI plugin: %s", cniPlugin)
				if m, ok := common.CNIPluginsManifests[cniPlugin]; ok {
					log.Printf("[DEBUG] [KUBEADM] CNI plugin: %s", cniPlugin)
					manifest = m
				} else {
					panic("unknown CNI driver: should have been caught at the validation stage")
				}
			}
		}
	}

	if len(manifest) == 0 {
		return ssh.Message("no CNI driver is going to be loaded")
	}
	kubeconfig := getKubeconfig(d)
	if kubeconfig == "" {
		log.Printf("[DEBUG] [KUBEADM] will not load CNI driver as no 'config_path' has been specified")
		return ssh.Message("ERROR: will not load CNI driver as no 'config_path' has been specified")
	}
	return ssh.DoLocalKubectlApply(kubeconfig, []string{manifest})
}

// doLoadManifests loads some extra manifests
func doLoadManifests(d *schema.ResourceData) ssh.ApplyFunc {
	manifestsOpt, ok := d.GetOk("manifests")
	if !ok {
		return ssh.EmptyAction()
	}
	manifests := []string{}
	for _, v := range manifestsOpt.([]interface{}) {
		manifests = append(manifests, v.(string))
	}

	kubeconfig := getKubeconfig(d)
	if kubeconfig == "" {
		return ssh.Message("ERROR: will not load manifests as no 'config_path' has been specified")
	}
	return ssh.DoLocalKubectlApply(kubeconfig, manifests)
}

// doLoadHelm loads Helm (if enabled)
func doLoadHelm(d *schema.ResourceData) ssh.ApplyFunc {
	opt, ok := d.GetOk("config.helm_enabled")
	if !ok {
		return ssh.Message("Helm will not be loaded")
	}
	enabled, err := strconv.ParseBool(opt.(string))
	if err != nil {
		panic("couold not parse helm_enabled in provisioner")
	}
	if !enabled {
		return ssh.Message("Helm will not be loaded")
	}
	if common.DefHelmManifest == "" {
		return ssh.Message("no manifest for Helm: Helm will not be loaded")
	}
	kubeconfig := getKubeconfig(d)
	if kubeconfig == "" {
		return ssh.Fatal("cannot not load Helm: no 'config_path' has been specified")
	}
	return ssh.DoLocalKubectlApply(kubeconfig, []string{common.DefHelmManifest})
}

// doLoadDashboard loads the dashboard (if enabled)
func doLoadDashboard(d *schema.ResourceData) ssh.ApplyFunc {
	opt, ok := d.GetOk("config.dashboard_enabled")
	if !ok {
		return ssh.Message("the Dashboard will not be loaded")
	}
	enabled, err := strconv.ParseBool(opt.(string))
	if err != nil {
		panic("couold not parse dashboard_enabled in provisioner")
	}
	if !enabled {
		return ssh.Message("the Dashboard will not be loaded")
	}
	if common.DefDashboardManifest == "" {
		return ssh.Message("no manifest for Dashboard: the Dashboard will not be loaded")
	}
	kubeconfig := getKubeconfig(d)
	if kubeconfig == "" {
		return ssh.Fatal("cannot not load the Dashboard: no 'config_path' has been specified")
	}
	return ssh.DoLocalKubectlApply(kubeconfig, []string{common.DefDashboardManifest})
}