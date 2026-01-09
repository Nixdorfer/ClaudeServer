package main

import (
	"gopkg.in/yaml.v3"
	"os"
)

type VersionChange struct {
	Version string   `yaml:"version" json:"version"`
	Time    string   `yaml:"time" json:"time"`
	Content []string `yaml:"content" json:"content"`
}

type VersionChangesYAML struct {
	Versions []VersionChange `yaml:"versions"`
}

type VersionChanges map[string]VersionChange

func LoadVersionChanges(filepath string) (VersionChanges, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	var changesYAML VersionChangesYAML
	err = yaml.Unmarshal(data, &changesYAML)
	if err != nil {
		return nil, err
	}
	changes := make(VersionChanges)
	for _, v := range changesYAML.Versions {
		changes[v.Version] = VersionChange{
			Version: v.Version,
			Time:    v.Time,
			Content: v.Content,
		}
	}
	return changes, nil
}

func GetLatestVersion(changes VersionChanges) string {
	latestVersion := ""
	for version := range changes {
		if version > latestVersion {
			latestVersion = version
		}
	}
	return latestVersion
}
