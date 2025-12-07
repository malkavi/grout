package ui

import gaba "github.com/UncleJunVIP/gabagool/v2/pkg/gabagool"

// ScreenResult wraps the output value and exit code from a screen
type ScreenResult[T any] struct {
	Value    T
	ExitCode gaba.ExitCode
}

// Success creates a successful screen result
func Success[T any](value T) ScreenResult[T] {
	return ScreenResult[T]{
		Value:    value,
		ExitCode: gaba.ExitCodeSuccess,
	}
}

// Back creates a back/cancel screen result
func Back[T any](value T) ScreenResult[T] {
	return ScreenResult[T]{
		Value:    value,
		ExitCode: gaba.ExitCodeBack,
	}
}

// WithCode creates a screen result with a specific exit code
func WithCode[T any](value T, code gaba.ExitCode) ScreenResult[T] {
	return ScreenResult[T]{
		Value:    value,
		ExitCode: code,
	}
}
