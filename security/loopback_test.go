package security

import "testing"

func TestIsLoopbackHost(t *testing.T) {
	allowed := []string{
		"localhost",
		"LOCALHOST",
		"127.0.0.1",
		"127.0.0.2",
		"::1",
		"::ffff:127.0.0.1",
		"iam.localhost",
		"iam.plateau.localhost",
		"ADMIN.Plateau.LocalHost",
	}
	for _, host := range allowed {
		if !IsLoopbackHost(host) {
			t.Errorf("IsLoopbackHost(%q) = false, want true", host)
		}
	}

	rejected := []string{
		"",
		"   ",
		".localhost",
		"..localhost",
		"foo..localhost",
		".iam.plateau.localhost",
		"localhost.",
		"evil-localhost.com",
		"iam.localhost.evil.com",
		"192.168.1.1",
		"10.0.0.1",
	}
	for _, host := range rejected {
		if IsLoopbackHost(host) {
			t.Errorf("IsLoopbackHost(%q) = true, want false", host)
		}
	}
}
