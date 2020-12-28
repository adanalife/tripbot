package helpers

import "math/rand"

const randCharset = "abcdefghijklmnopqrstuvwxyz" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// var seededRand *rand.Rand = rand.New(
// 	rand.NewSource(time.Now().UnixNano()))

// https://www.calhoun.io/creating-random-strings-in-go/
func stringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = randCharset[rand.Intn(len(charset))]
	}
	return string(b)
}

func RandString(length int) string {
	return stringWithCharset(length, randCharset)
}
