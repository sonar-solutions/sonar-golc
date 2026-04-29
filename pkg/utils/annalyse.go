package utils

import (
	"bufio"
	"os"
	"strings"
)

type ExclusionList struct {
	Projects map[string]bool
	Repos    map[string]bool
}

func NewExclusionList(projects, repos []string) *ExclusionList {
	el := &ExclusionList{
		Projects: make(map[string]bool),
		Repos:    make(map[string]bool),
	}
	for _, p := range projects {
		el.Projects[p] = true
	}
	for _, r := range repos {
		el.Repos[r] = true
	}
	return el
}

func LoadExclusionList(filename string) (*ExclusionList, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	exclusionList := &ExclusionList{
		Projects: make(map[string]bool),
		Repos:    make(map[string]bool),
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "/")
		if len(parts) == 1 {
			// Exclusion de projet
			exclusionList.Projects[parts[0]] = true
		} else if len(parts) == 2 {
			// Exclusion de répertoire
			exclusionList.Repos[line] = true
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return exclusionList, nil
}
