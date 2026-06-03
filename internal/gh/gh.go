package gh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/kakkoyun/af/internal/sandbox"
)

// ErrNoPR reports that gh pr view could not resolve a PR for the
// current context. ADR-073 §4 specifies this as a hard error from
// af review with a remediation hint.
var ErrNoPR = errors.New("gh: no pull request resolved")

// ErrEmptyDiff reports that gh pr diff returned no content. ADR-073
// §5 maps this to errReviewEmptyDiff in cmd/af.
var ErrEmptyDiff = errors.New("gh: pr diff is empty")

// ErrCommandFailed wraps a non-zero exit from the gh CLI. Callers can
// errors.Is against it to detect any gh failure mode.
var ErrCommandFailed = errors.New("gh: command failed")

// PRMeta describes the PR returned by `gh pr view --json`.
type PRMeta struct {
	Title       string `json:"title"`
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
	Number      int    `json:"number"`
}

// ViewPR runs `gh pr view <n> --json number,title,headRefName,baseRefName`
// when number > 0, or `gh pr view --json …` for the current branch
// when number == 0. Returns ErrNoPR when gh reports no PR for the
// branch (most common when the branch hasn't been pushed yet).
func ViewPR(ctx context.Context, runner sandbox.Runner, number int) (PRMeta, error) {
	args := []string{"pr", "view"}
	if number > 0 {
		args = append(args, strconv.Itoa(number))
	}
	args = append(args, "--json", "number,title,headRefName,baseRefName")
	output, err := run(ctx, runner, args)
	if err != nil {
		if isNoPRError(err) {
			return PRMeta{}, ErrNoPR
		}
		return PRMeta{}, err
	}
	var meta PRMeta
	parseErr := json.Unmarshal(output, &meta)
	if parseErr != nil {
		return PRMeta{}, fmt.Errorf("parse gh pr view output: %w", parseErr)
	}
	if meta.Number == 0 {
		return PRMeta{}, ErrNoPR
	}
	return meta, nil
}

// DiffPR runs `gh pr diff <n>` and returns the raw unified diff. An
// empty diff returns (string, ErrEmptyDiff) so callers can hard-error
// per ADR-073 §5 without parsing the body.
func DiffPR(ctx context.Context, runner sandbox.Runner, number int) (string, error) {
	if number <= 0 {
		return "", fmt.Errorf("%w: number must be > 0", ErrNoPR)
	}
	args := []string{"pr", "diff", strconv.Itoa(number)}
	output, err := run(ctx, runner, args)
	if err != nil {
		return "", err
	}
	diff := string(output)
	if strings.TrimSpace(diff) == "" {
		return "", ErrEmptyDiff
	}
	return diff, nil
}

func run(ctx context.Context, runner sandbox.Runner, args []string) ([]byte, error) {
	if runner == nil {
		runner = sandbox.ExecRunner{}
	}
	output, err := runner.Run(ctx, sandbox.Command{Name: "gh", Args: args})
	if err != nil {
		return output, fmt.Errorf("%w: %w", ErrCommandFailed, err)
	}
	return output, nil
}

// isNoPRError inspects an error message for the gh "no pull requests
// found" string. gh's exit message varies by version; this matches the
// stable substrings observed in 2.40+.
func isNoPRError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no pull requests") ||
		strings.Contains(msg, "no open pull requests") ||
		strings.Contains(msg, "could not resolve to a pullrequest")
}
