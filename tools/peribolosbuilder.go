package main

/*
TODO:
	Handle this copyright properly - the basis was the Kubernets script, but a lot of things have changed
	Not Sure how to best reflect on this :)

Copyright 2018 The Kubernetes Authors.
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
import (
	"flag"
	"fmt"
	"golang.org/x/exp/maps"
	"k8s.io/test-infra/prow/config/org"
	"k8s.io/test-infra/prow/github"
	"os"
	"path/filepath"

	"github.com/ghodss/yaml"
	"github.com/sirupsen/logrus"
)

const (
	Maintainers string = "maintainers"
	Approvers   string = "approvers"
)

type options struct {
	config string
}
type Group struct {
	Repos       []string `json:"repos,omitempty"`
	Maintainers []string `json:"maintainers,omitempty"`
	Approvers   []string `json:"approvers,omitempty"`
}

func main() {
	o := options{}
	flag.StringVar(&o.config, "config", "config", "")
	flag.Parse()

	cfg, err := loadOrgs(o)
	if err != nil {
		logrus.Fatalf("Failed to load orgs: %v", err)
	}
	pc := org.FullConfig{
		Orgs: cfg,
	}
	out, err := yaml.Marshal(pc)
	if err != nil {
		logrus.Fatalf("Failed to marshal orgs: %v", err)
	}
	fmt.Println(string(out))
}

func unmarshal(path string) (*org.Config, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read: %v", err)
	}
	var cfg org.Config
	if err := yaml.Unmarshal(buf, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal: %v", err)
	}
	return &cfg, nil
}

func unmarshalGroup(path string) (*Group, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read: %v", err)
	}
	var cfg Group
	if err := yaml.Unmarshal(buf, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal: %v", err)
	}
	return &cfg, nil
}

func loadOrgs(o options) (map[string]org.Config, error) {
	config := map[string]org.Config{}
	entries, err := os.ReadDir(o.config)
	if err != nil {
		return nil, fmt.Errorf("error in %s: %v", o.config, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		path := o.config + "/" + name + "/org.yaml"
		cfg, err := unmarshal(path)
		if err != nil {
			return nil, fmt.Errorf("error in %s: %v", path, err)
		}
		if cfg.Teams == nil {
			cfg.Teams = map[string]org.Team{}
		}
		prefix := filepath.Dir(path)
		allRepos := map[string]bool{}
		err = filepath.Walk(prefix, func(path string, info os.FileInfo, err error) error {
			switch {
			case path == prefix:
				return nil // Skip base dir
			case info.IsDir() && filepath.Dir(path) != prefix:
				logrus.Infof("Skipping %s and its children", path)
				return filepath.SkipDir // Skip prefix/foo/bar/ dirs
			case !info.IsDir() && filepath.Dir(path) == prefix:
				return nil // Ignore prefix/foo files
			case filepath.Base(path) == "teams.yaml":
				teams, repos, err := generateGroupConfig(path)

				if err != nil {
					return err
				}

				maps.Copy(cfg.Teams, teams)
				maps.Copy(allRepos, repos)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("merge teams %s: %v", path, err)
		}
		maintainers := getGlobalTeam(cfg, Maintainers)
		approvers := getGlobalTeam(cfg, Approvers)

		for name := range allRepos {
			maintainers.Repos[name] = github.Maintain
			approvers.Repos[name] = github.Triage
		}

		cfg.Teams[Maintainers] = maintainers
		cfg.Teams[Approvers] = maintainers
		config[name] = *cfg
	}
	return config, nil
}

func getGlobalTeam(cfg *org.Config, teamName string) org.Team {
	team, ok := cfg.Teams[teamName]
	if !ok {
		team = org.Team{}
	}
	if team.Repos == nil {
		team.Repos = map[string]github.RepoPermissionLevel{}
	}
	return team
}

func generateGroupConfig(path string) (map[string]org.Team, map[string]bool, error) {
	groupCfg, err := unmarshalGroup(path)
	if err != nil {
		return nil, nil, fmt.Errorf("error in %s: %v", path, err)
	}

	maintainers := org.Team{
		Members: groupCfg.Maintainers,
		Repos:   map[string]github.RepoPermissionLevel{},
	}
	approvers := org.Team{
		Members: groupCfg.Approvers,
		Repos:   map[string]github.RepoPermissionLevel{},
	}

	groupRepos := map[string]bool{}

	// adding repos to the all repos list
	for _, repo := range groupCfg.Repos {
		maintainers.Repos[repo] = github.Maintain
		approvers.Repos[repo] = github.Triage
		groupRepos[repo] = true
	}

	group := filepath.Base(filepath.Dir(path))
	teams := map[string]org.Team{}
	teams[group+"-"+Maintainers] = maintainers
	teams[group+"-"+Approvers] = approvers
	return teams, groupRepos, nil
}
