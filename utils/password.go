package utils

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {

	hashPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	if err != nil {
		return "", fmt.Errorf("failed to hash password %s\n", err)
	}

	return string(hashPassword), nil
}

func CheckPassword(password, hashPassword string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashPassword), []byte(password))
}
func ValidatePassword(password string, passwordMinLength int) error {
	errWeakPassword := errors.New("password does not meet requirements")
	if len(password) < passwordMinLength {
		return fmt.Errorf("%w: password must be at least %d characters", errWeakPassword, passwordMinLength)
	}
	return nil
}
