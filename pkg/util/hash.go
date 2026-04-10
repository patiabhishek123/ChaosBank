package util

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func HashRequest(data interface{}) (string, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:]), nil
}
