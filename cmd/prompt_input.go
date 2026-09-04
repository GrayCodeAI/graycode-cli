package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/engine"
)

var errNoInteractivePromptInput = errors.New("no interactive terminal available for permission prompt")

type promptInput struct {
	reader *bufio.Reader
	close  func()
}

func openPromptInput() promptInput {
	if isStdinTerminal() {
		return promptInput{
			reader: bufio.NewReader(os.Stdin),
			close:  func() {},
		}
	}

	tty, err := os.Open("/dev/tty")
	if err != nil {
		return promptInput{close: func() {}}
	}
	return promptInput{
		reader: bufio.NewReader(tty),
		close: func() {
			_ = tty.Close()
		},
	}
}

func (p promptInput) readLine(prompt string) (string, error) {
	if p.reader == nil {
		return "", errNoInteractivePromptInput
	}
	_, _ = fmt.Fprint(os.Stderr, prompt)
	answer, err := p.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(answer), nil
}

func configureInteractivePrompts(sess *engine.Session, input promptInput) {
	sess.PermSvc().SetPermissionFn(func(req engine.PermissionRequest) {
		answer, err := input.readLine(fmt.Sprintf("\nAllow %s: %s [y/N] ", req.ToolName, req.Summary))
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "\nAllow %s: %s [y/N] (denied: %v)\n", req.ToolName, req.Summary, err)
			req.Response <- false
			return
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		req.Response <- answer == "y" || answer == "yes"
	})
	sess.SetAskUserFn(func(question string) (string, error) {
		return input.readLine(fmt.Sprintf("\n%s\n> ", question))
	})
}
