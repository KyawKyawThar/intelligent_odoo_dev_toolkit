// Package utils provides utility functions for the application.
package utils

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword returns the bcrypt hash of the password.
func HashPassword(password string) (string, error) {

	hashPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	return string(hashPassword), nil
}

// CheckPassword checks if the provided password is correct or not.
func CheckPassword(password, hashPassword string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashPassword), []byte(password))
}

// ValidatePassword validates the password.
func ValidatePassword(password string, passwordMinLength int) error {
	errWeakPassword := errors.New("password does not meet requirements")
	if len(password) < passwordMinLength {
		return fmt.Errorf("%w: password must be at least %d characters", errWeakPassword, passwordMinLength)
	}
	return nil
}
