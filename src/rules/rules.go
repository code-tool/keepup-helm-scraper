package rules

import (
	"fmt"
	"os"
	"regexp"

	"go.yaml.in/yaml/v2"
)

type DetectionRuleYaml struct {
	ApplicationName string `yaml:"applicationName"`
	VersionRegex    string `yaml:"versionRegex"`
	DetectionRegex  string `yaml:"detectionRegex"`
}

type DetectionConfigFile struct {
	DockerImages []DetectionRuleYaml `yaml:"docker"`
}

type Rule struct {
	ApplicationName string
	VersionRegex    *regexp.Regexp
	DetectionRegex  *regexp.Regexp
}

type DetectedComponent struct {
	Kind    string
	Name    string
	Version string
}

func LoadRules(path string) ([]Rule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var rf DetectionConfigFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return nil, err
	}

	var rules []Rule
	for _, r := range rf.DockerImages {
		detectRe, err := regexp.Compile(r.DetectionRegex)
		if err != nil {
			return nil, fmt.Errorf("invalid detection regex for %s: %w", r.ApplicationName, err)
		}

		versionRe, err := regexp.Compile(r.VersionRegex)
		if err != nil {
			return nil, fmt.Errorf("invalid version regex for %s: %w", r.ApplicationName, err)
		}

		rules = append(rules, Rule{
			ApplicationName: r.ApplicationName,
			DetectionRegex:  detectRe,
			VersionRegex:    versionRe,
		})
	}

	return rules, nil
}
