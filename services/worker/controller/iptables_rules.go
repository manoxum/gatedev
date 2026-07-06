package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// deleteRulesByComment apaga, em loop, toda regra do chain informado
// que contenha o comentario dado - mesmo padrao de
// remove_iptables_jump em interfaces.sh (repete ate o -D falhar). table
// vazia = tabela default (filter), usado pelo bloqueio de trafego por
// credito em BINDNET-HOTSPOT (ver traffic_block.go); table "mangle" e
// usado pelo shaping (ver shaping_iptables.go).
func deleteRulesByComment(table, chain, comment string) {
	listArgs := listChainArgs(table, chain)
	output, err := exec.Command("iptables", listArgs...).CombinedOutput()
	if err != nil {
		return
	}
	for {
		lineNumber := findRuleLineByComment(string(output), comment)
		if lineNumber == "" {
			return
		}
		deleteArgs := deleteRuleArgs(table, chain, lineNumber)
		if err := runIptables(deleteArgs...); err != nil {
			return
		}
		output, err = exec.Command("iptables", listArgs...).CombinedOutput()
		if err != nil {
			return
		}
	}
}

func listChainArgs(table, chain string) []string {
	args := []string{"-w"}
	if table != "" {
		args = append(args, "-t", table)
	}
	return append(args, "-L", chain, "--line-numbers", "-n")
}

func deleteRuleArgs(table, chain, lineNumber string) []string {
	var args []string
	if table != "" {
		args = append(args, "-t", table)
	}
	return append(args, "-D", chain, lineNumber)
}

func deleteRulesByCommentTarget(comment, target string) {
	output, err := exec.Command("iptables", "-w", "-t", "mangle", "-L", shapingChain, "--line-numbers", "-n").CombinedOutput()
	if err != nil {
		return
	}
	for {
		lineNumber := findRuleLineByCommentTarget(string(output), comment, target)
		if lineNumber == "" {
			return
		}
		if err := runIptables("-t", "mangle", "-D", shapingChain, lineNumber); err != nil {
			return
		}
		output, err = exec.Command("iptables", "-w", "-t", "mangle", "-L", shapingChain, "--line-numbers", "-n").CombinedOutput()
		if err != nil {
			return
		}
	}
}

func findRuleLineByComment(listing, comment string) string {
	for _, line := range strings.Split(listing, "\n") {
		if strings.Contains(line, "/* "+comment+" */") {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				return fields[0]
			}
		}
	}
	return ""
}

func findRuleLineByCommentTarget(listing, comment, target string) string {
	for _, line := range strings.Split(listing, "\n") {
		if !strings.Contains(line, "/* "+comment+" */") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 1 && fields[1] == target {
			return fields[0]
		}
	}
	return ""
}

func runIptables(args ...string) error {
	output, err := exec.Command("iptables", append([]string{"-w"}, args...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func iptablesCheck(args ...string) error {
	_, err := exec.Command("iptables", append([]string{"-w"}, args...)...).CombinedOutput()
	return err
}
