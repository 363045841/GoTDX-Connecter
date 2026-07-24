package client

import (
	"errors"
	"testing"

	"github.com/bensema/gotdx"
)

func TestReprobeKeepsCurrentClientWhenReplacementConnectionFails(t *testing.T) {
	originalInstance := instance
	originalBuildClient := buildClientForReprobe
	originalConnectClient := connectClientForReprobe
	t.Cleanup(func() {
		instance = originalInstance
		buildClientForReprobe = originalBuildClient
		connectClientForReprobe = originalConnectClient
	})

	current := gotdx.New()
	replacement := gotdx.New()
	instance = current
	buildClientForReprobe = func() *gotdx.Client { return replacement }
	connectClientForReprobe = func(*gotdx.Client) error { return errors.New("connect failed") }

	if err := Reprobe(); err == nil {
		t.Fatal("Reprobe succeeded when replacement connection failed")
	}
	if instance != current {
		t.Fatal("Reprobe replaced the current client after connection failure")
	}
}

func TestReprobeSwapsClientAfterReplacementConnects(t *testing.T) {
	originalInstance := instance
	originalBuildClient := buildClientForReprobe
	originalConnectClient := connectClientForReprobe
	t.Cleanup(func() {
		instance = originalInstance
		buildClientForReprobe = originalBuildClient
		connectClientForReprobe = originalConnectClient
	})

	current := gotdx.New()
	replacement := gotdx.New()
	instance = current
	buildClientForReprobe = func() *gotdx.Client { return replacement }
	connectClientForReprobe = func(*gotdx.Client) error { return nil }

	if err := Reprobe(); err != nil {
		t.Fatalf("Reprobe failed: %v", err)
	}
	if instance != replacement {
		t.Fatal("Reprobe did not install the connected replacement client")
	}
}

func TestReprobeMainDoesNotRequireExtendedConnection(t *testing.T) {
	originalInstance := instance
	originalBuildClient := buildClientForReprobe
	originalConnectMain := connectMainForReprobe
	t.Cleanup(func() {
		instance = originalInstance
		buildClientForReprobe = originalBuildClient
		connectMainForReprobe = originalConnectMain
	})

	current := gotdx.New()
	replacement := gotdx.New()
	instance = current
	buildClientForReprobe = func() *gotdx.Client { return replacement }
	connectMainForReprobe = func(*gotdx.Client) error { return nil }

	if err := ReprobeMain(); err != nil {
		t.Fatalf("ReprobeMain failed: %v", err)
	}
	if instance != replacement {
		t.Fatal("ReprobeMain did not install the connected replacement client")
	}
}
