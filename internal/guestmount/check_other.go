//go:build !linux

package guestmount

func Check() error { return ErrMount }
