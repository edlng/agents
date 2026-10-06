package gate

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GPG signs with the local gpg keyring. Key selects the signing key; when it
// is empty, the key is resolved the way git resolves it for signed commits:
// user.signingkey, then user.email.
type GPG struct{ Key string }

func (g GPG) signingKey() (string, error) {
	if g.Key != "" {
		return g.Key, nil
	}
	for _, setting := range []string{"user.signingkey", "user.email"} {
		out, err := exec.Command("git", "config", "--get", setting).Output()
		if key := strings.TrimSpace(string(out)); err == nil && key != "" {
			return key, nil
		}
	}
	return "", fmt.Errorf("no signing key: pass --key or set git user.signingkey")
}

func (g GPG) Sign(data []byte) (string, string, error) {
	key, err := g.signingKey()
	if err != nil {
		return "", "", err
	}
	args := []string{"--armor", "--detach-sign", "--status-fd", "2", "--local-user", key, "--output", "-"}
	cmd := exec.Command("gpg", args...)
	cmd.Stdin = bytes.NewReader(data)
	var out, status bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &status
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("gpg sign: %v: %s", err, lastLines(status.String()))
	}
	// [GNUPG:] SIG_CREATED <type> <pk_algo> <hash_algo> <class> <timestamp> <fingerprint>
	fpr := statusField(status.String(), "SIG_CREATED", 5)
	if fpr == "" {
		return "", "", fmt.Errorf("gpg sign: no SIG_CREATED status")
	}
	return out.String(), fpr, nil
}

func (g GPG) Verify(data []byte, signature string) (string, error) {
	dir, err := os.MkdirTemp("", "delegate-verify-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	sig := filepath.Join(dir, "decision.asc")
	if err := os.WriteFile(sig, []byte(signature), 0o600); err != nil {
		return "", err
	}
	cmd := exec.Command("gpg", "--status-fd", "1", "--verify", sig, "-")
	cmd.Stdin = bytes.NewReader(data)
	var status, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &status, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gpg verify: %v: %s", err, lastLines(stderr.String()))
	}
	// [GNUPG:] VALIDSIG <fingerprint> ...
	fpr := statusField(status.String(), "VALIDSIG", 0)
	if fpr == "" || !strings.Contains(status.String(), "[GNUPG:] GOODSIG ") {
		return "", fmt.Errorf("gpg verify: signature is not valid")
	}
	return fpr, nil
}

func statusField(status, keyword string, index int) string {
	for _, line := range strings.Split(status, "\n") {
		fields := strings.Fields(line)
		if len(fields) > index+2 && fields[0] == "[GNUPG:]" && fields[1] == keyword {
			return fields[index+2]
		}
	}
	return ""
}

func lastLines(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > 3 {
		lines = lines[len(lines)-3:]
	}
	return strings.Join(lines, " | ")
}
