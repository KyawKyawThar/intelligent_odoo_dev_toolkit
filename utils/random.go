package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/google/uuid"
)

const alphabet = "abcdefghijklmnopqrstuvwxyz"

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

func RandomInteger(min, max int64) int64 {
	if min > max {
		panic("min cannot be greater than max")
	}

	n, err := rand.Int(rand.Reader, big.NewInt(max-min+1))
	if err != nil {
		panic(err)
	}
	return n.Int64() + min
}

func RandomOwner() string {
	return RandomString(7)
}
func RandomEmail() string {
	return fmt.Sprintf("%s@%s.com", RandomString(8), RandomString(5))
}

func RandomSlug() string {
	return fmt.Sprintf("%s-%s", RandomString(4), RandomString(4))
}

func RandomUUID() uuid.UUID {

	return uuid.New()
}
