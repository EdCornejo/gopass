package action

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/justwatchcom/gopass/store/secret"
	"github.com/justwatchcom/gopass/utils/ctxutil"
	"github.com/urfave/cli"
)

// Insert a string as content to a secret file
func (s *Action) Insert(ctx context.Context, c *cli.Context) error {
	echo := c.Bool("echo")
	multiline := c.Bool("multiline")
	force := c.Bool("force")

	confirm := s.confirmRecipients
	if force {
		confirm = nil
	}

	name := c.Args().Get(0)
	if name == "" {
		return s.exitError(ctx, ExitNoName, nil, "Usage: %s insert name", s.Name)
	}

	key := c.Args().Get(1)

	var content []byte
	var fromStdin bool

	info, err := os.Stdin.Stat()
	if err != nil {
		return s.exitError(ctx, ExitIO, err, "failed to stat stdin: %s", err)
	}

	// if content is piped to stdin, read and save it
	if info.Mode()&os.ModeCharDevice == 0 {
		fromStdin = true
		buf := &bytes.Buffer{}

		if written, err := io.Copy(buf, os.Stdin); err != nil {
			return s.exitError(ctx, ExitIO, err, "failed to copy after %d bytes: %s", written, err)
		}

		content = buf.Bytes()
	}

	// update to a single YAML entry
	if key != "" {
		if !fromStdin {
			pw, err := s.askForString(name+":"+key, "")
			if err != nil {
				return s.exitError(ctx, ExitIO, err, "failed to ask for user input: %s", err)
			}
			content = []byte(pw)
		}

		sec := secret.New("", "")
		if s.Store.Exists(name) {
			var err error
			sec, err = s.Store.Get(ctx, name)
			if err != nil {
				return s.exitError(ctx, ExitEncrypt, err, "failed to set key '%s' of '%s': %s", key, name, err)
			}
		}
		if err := sec.SetValue(key, string(content)); err != nil {
			return s.exitError(ctx, ExitEncrypt, err, "failed to set key '%s' of '%s': %s", key, name, err)
		}
		if err := s.Store.Set(ctx, name, sec, "Inserted YAML value from STDIN"); err != nil {
			return s.exitError(ctx, ExitEncrypt, err, "failed to set key '%s' of '%s': %s", key, name, err)
		}
		return nil
	}

	if fromStdin {
		sec, err := secret.Parse(content)
		if err != nil {
			return s.exitError(ctx, ExitEncrypt, err, "failed to set '%s': %s", name, err)
		}
		if err := s.Store.SetConfirm(ctx, name, sec, "Read secret from STDIN", confirm); err != nil {
			return s.exitError(ctx, ExitEncrypt, err, "failed to set '%s': %s", name, err)
		}
		return nil
	}

	if !force { // don't check if it's force anyway
		if s.Store.Exists(name) && !s.askForConfirmation(ctx, fmt.Sprintf("An entry already exists for %s. Overwrite it?", name)) {
			return s.exitError(ctx, ExitAborted, nil, "not overwriting your current secret")
		}
	}

	// if multi-line input is requested start an editor
	if multiline && ctxutil.IsInteractive(ctx) {
		content, err := s.editor(ctx, []byte{})
		if err != nil {
			return s.exitError(ctx, ExitUnknown, err, "failed to start editor: %s", err)
		}
		sec, err := secret.Parse(content)
		if err != nil {
			return s.exitError(ctx, ExitUnknown, err, "failed to parse secret: %s", err)
		}
		if err := s.Store.SetConfirm(ctx, name, sec, fmt.Sprintf("Inserted user supplied password with %s", os.Getenv("EDITOR")), confirm); err != nil {
			return s.exitError(ctx, ExitEncrypt, err, "failed to store secret '%s': %s", name, err)
		}
		return nil
	}

	// if echo mode is requested use a simple string input function
	var promptFn func(context.Context, string) (string, error)
	if echo {
		promptFn = func(ctx context.Context, prompt string) (string, error) {
			return s.askForString(prompt, "")
		}
	}

	pw, err := s.askForPassword(ctx, name, promptFn)
	if err != nil {
		return s.exitError(ctx, ExitIO, err, "failed to ask for password: %s", err)
	}

	sec := secret.New(pw, "")
	printAuditResult(sec.Password())

	if err := s.Store.SetConfirm(ctx, name, sec, "Inserted user supplied password", confirm); err != nil {
		return s.exitError(ctx, ExitEncrypt, err, "failed to write secret '%s': %s", name, err)
	}
	return nil
}
