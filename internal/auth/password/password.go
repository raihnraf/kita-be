package password

import (
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	cost int
}

func NewService() *Service {
	return &Service{cost: bcrypt.DefaultCost}
}

func (s *Service) Hash(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (s *Service) Verify(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
