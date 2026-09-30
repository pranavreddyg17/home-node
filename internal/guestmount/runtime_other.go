//go:build !linux

package guestmount

func CheckRuntime() error { return ErrMount }
