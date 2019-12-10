// Copyright 2019 Antrea Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package providers

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"runtime"

	"github.com/kevinburke/ssh_config"
	"golang.org/x/crypto/ssh"

	"github.com/vmware-tanzu/antrea/test/e2e/providers/exec"
)

func vagrantPath() (string, error) {
	// TODO: find a better way to get this path?
	_, fileName, _, ok := runtime.Caller(1)
	if !ok {
		return "", fmt.Errorf("cannot locate caller information")
	}
	return path.Join(path.Dir(path.Dir(fileName)), "infra", "vagrant"), nil
}

func importConfig() (*ssh_config.Config, error) {
	vagrantPath, err := vagrantPath()
	if err != nil {
		return nil, fmt.Errorf("cannot compute path to infra/vagrant")
	}
	configPath := path.Join(vagrantPath, "ssh-config")
	f, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open ssh-config file: %v", err)
	}
	config, err := ssh_config.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("cannot decode ssh-config file: %v", err)
	}
	return config, nil
}

func convertConfig(inConfig *ssh_config.Config, name string) (string, *ssh.ClientConfig, error) {
	if inConfig == nil {
		return "", nil, fmt.Errorf("input config is nil")
	}

	getFromKeyStrict := func(key string) (string, error) {
		v, err := inConfig.Get(name, key)
		if err != nil {
			return "", fmt.Errorf("error when retrieving '%s' for '%s' in SSH config: %v", key, name, err)
		}
		if v == "" {
			return "", fmt.Errorf("unable to find '%s' for '%s' in SSH config", key, name)
		}
		return v, nil
	}

	keyList := []string{"HostName", "Port", "User", "IdentityFile"}
	values := make(map[string]string)

	for _, key := range keyList {
		if value, err := getFromKeyStrict(key); err != nil {
			return "", nil, err
		} else {
			values[key] = value
		}
	}

	identityFile := values["IdentityFile"]
	// Read the private key identified by identityFile.
	key, err := ioutil.ReadFile(identityFile)
	if err != nil {
		return "", nil, fmt.Errorf("unable to read private key from file '%s': %v", identityFile, err)
	}
	// Create the Signer for this private key.
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return "", nil, fmt.Errorf("unable to parse private key from file '%s': %v", identityFile, err)
	}

	config := &ssh.ClientConfig{
		User:            values["User"],
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	host := fmt.Sprintf("%s:%s", values["HostName"], values["Port"])
	return host, config, nil
}

func GetSSHConfig(name string) (string, *ssh.ClientConfig, error) {
	// We convert the Vagrant ssh config file (generated by "vagrant ssh-config"), which is an
	// OpenSSH config file, to an instance of ssh.ClientConfig.
	sshConfig, err := importConfig()
	if err != nil {
		return "", nil, err
	}
	return convertConfig(sshConfig, name)
}

type VagrantProvider struct{}

func (provider *VagrantProvider) RunCommandOnNode(nodeName string, cmd string) (
	code int, stdout io.Reader, stderr io.Reader, err error,
) {
	host, config, err := GetSSHConfig(nodeName)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("error when retrieving SSH config for node '%s': %v", nodeName, err)
	}
	return exec.RunSSHCommand(host, config, cmd)
}

func (provider *VagrantProvider) GetKubeconfigPath() (string, error) {
	vagrantPath, err := vagrantPath()
	if err != nil {
		return "", fmt.Errorf("cannot compute path to infra/vagrant")
	}
	kubeconfigPath := path.Join(vagrantPath, "playbook", "kube", "config")
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		return "", fmt.Errorf("Kubeconfig file not found at expected location '%s'", kubeconfigPath)
	}
	return kubeconfigPath, nil
}

// NewVagrantProvider returns an implementation of ProviderInterface which is suitable for a
// Kubernetes test cluster created with the provided Vagrantfile.
// configPath is unused for the vagrant provider
func NewVagrantProvider(configPath string) (ProviderInterface, error) {
	return &VagrantProvider{}, nil
}
