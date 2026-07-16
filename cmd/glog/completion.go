package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/alecthomas/kong"
)

const shellBash = "bash"

type completionCommand struct {
	Shell          string `arg:"" enum:"bash,zsh,fish,powershell" help:"Shell type: ${enum}."`
	NoDescriptions bool   `name:"no-descriptions" help:"Disable completion descriptions for compatibility."`
}

func (c completionCommand) Run(app *kong.Kong) error {
	return writeCompletion(app.Stdout, c.Shell, completionCandidates(app.Model.Node))
}

func completionCandidates(root *kong.Node) []string {
	candidates := map[string]struct{}{}
	var visit func(*kong.Node)
	visit = func(node *kong.Node) {
		if node.Hidden {
			return
		}
		addCompletionNodeCandidates(candidates, node)
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(root)

	result := make([]string, 0, len(candidates))
	for candidate := range candidates {
		result = append(result, candidate)
	}
	sort.Strings(result)
	return result
}

func addCompletionNodeCandidates(candidates map[string]struct{}, node *kong.Node) {
	if node.Type == kong.CommandNode && node.Name != "" {
		candidates[node.Name] = struct{}{}
		for _, alias := range node.Aliases {
			if alias != "" {
				candidates[alias] = struct{}{}
			}
		}
	}
	for _, flag := range node.Flags {
		if flag.Hidden {
			continue
		}
		candidates["--"+flag.Name] = struct{}{}
		if flag.Short != 0 {
			candidates["-"+string(flag.Short)] = struct{}{}
		}
	}
	for _, positional := range node.Positional {
		for _, value := range positional.EnumSlice() {
			if value != "" {
				candidates[value] = struct{}{}
			}
		}
	}
}

func writeCompletion(w io.Writer, shell string, candidates []string) error {
	words := strings.Join(candidates, " ")
	var script string
	switch shell {
	case shellBash:
		script = fmt.Sprintf(`# bash completion for glog
_glog_completion() {
    local cur="${COMP_WORDS[COMP_CWORD]}"
    COMPREPLY=( $(compgen -W %q -- "$cur") )
}
complete -F _glog_completion glog
`, words)
	case "zsh":
		script = fmt.Sprintf("#compdef glog\n_arguments '*: :((%s))'\n", words)
	case "fish":
		script = fmt.Sprintf("complete -c glog -f -a %q\n", words)
	case "powershell":
		script = fmt.Sprintf(`Register-ArgumentCompleter -Native -CommandName glog -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    %q.Split(' ') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
    }
}
`, words)
	default:
		return fmt.Errorf("unsupported shell %q", shell)
	}
	_, err := io.WriteString(w, script)
	return err
}
