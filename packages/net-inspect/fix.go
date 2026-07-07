package main

import (
	"bufio"
	"fmt"
	"strings"
)

func confirmFix(promptText string, scanner *bufio.Scanner) bool {
	for {
		fmt.Print(promptText)
		if !scanner.Scan() {
			return false
		}
		input := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if input == "yes" || input == "y" || input == "是" {
			return true
		}
		if input == "no" || input == "n" || input == "否" {
			return false
		}
		fmt.Println(getT(FixConfirmHint))
	}
}

