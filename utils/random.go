package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/google/uuid"
)

const alphabet = "abcdefghijklmnopqrstuvwxyz"

// RandomString generates a random string of a given length.
func RandomString(maxLength int) string {
	b := make([]byte, maxLength)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}

	for i := range b {
		b[i] = alphabet[b[i]%byte(len(alphabet))]
	}

	return string(b)
}

// RandomInteger generates a random integer between min and max.
func RandomInteger(minVal, maxVal int64) int64 {
	if minVal > maxVal {
		panic("min cannot be greater than max")
	}

	n, err := rand.Int(rand.Reader, big.NewInt(maxVal-minVal+1))
	if err != nil {
		panic(err)
	}
	return n.Int64() + minVal
}

// RandomOwner generates a random owner name.
func RandomOwner() string {
	return RandomString(7)
}

// RandomEmail generates a random email address.
func RandomEmail() string {
	return fmt.Sprintf("%s@%s.com", RandomString(8), RandomString(5))
}

// RandomSlug generates a random slug.
func RandomSlug() string {
	return fmt.Sprintf("%s-%s", RandomString(4), RandomString(4))
}

// RandomUUID generates a random UUID.
func RandomUUID() uuid.UUID {

	return uuid.New()
}
