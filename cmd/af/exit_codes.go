package main

import (
	"context"
	"errors"
	"strings"
)

const (
	exitOK          = 0
	exitGeneral     = 1
	exitUsage       = 64
	exitDataErr     = 65
	exitNoInput     = 66
	exitInterrupted = 130
)

func exitCodeForError(err error) int {
	if err == nil {
		return exitOK
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, errSessionPickerInterrupted):
		return exitInterrupted
	case errors.Is(err, errSessionResolutionNoInput), errors.Is(err, errProxyNoState), errors.Is(err, errStackNoState):
		return exitNoInput
	case errors.Is(err, errPRRefreshNoPR), errors.Is(err, errPRAIEmptyDiff), errors.Is(err, errReviewEmptyDiff), errors.Is(err, errReviewEmptyBody):
		return exitDataErr
	case isUsageError(err):
		return exitUsage
	default:
		return exitGeneral
	}
}

func isUsageError(err error) bool {
	message := err.Error()
	return strings.Contains(message, "unknown command") ||
		strings.Contains(message, "unknown flag") ||
		strings.Contains(message, "accepts ") ||
		errors.Is(err, errNoteAppendRequired) ||
		errors.Is(err, errStackParentRequired) ||
		errors.Is(err, errUnsupportedShell)
}
