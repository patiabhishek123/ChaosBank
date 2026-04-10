package util

import "github.com/google/uuid"

func GenerateRequestID() string {
	return uuid.New().String()
}

func ValidateRequestID(id string) bool {
	_, err := uuid.Parse(id)
	return err == nil
}
