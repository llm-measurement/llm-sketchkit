// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package main

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/frequentitems"
	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
)

func TestWindowConcurrencyAndIsolation(t *testing.T) {
	secret := testSecret(t)
	workers := make([][]event, 8)
	for i := range workers {
		for range 100 {
			workers[i] = append(workers[i], event{"research", 89}, event{"support", 12})
		}
	}
	var out bytes.Buffer
	if err := runWindow(secret, workers, []string{"support", "research"}, &out); err != nil {
		t.Fatal(err)
	}
	want := "distinct keys: 2\nresearch tokens=71200 bounds=[71200,71200]\nsupport tokens=9600 bounds=[9600,9600]\n"
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
	out.Reset()
	if err := runWindow(secret, nil, nil, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "distinct keys: 0\n" {
		t.Fatalf("window leaked state: %q", out.String())
	}
}

func TestWindowErrorsAndUnknownKeys(t *testing.T) {
	secret := testSecret(t)
	for _, tc := range []struct {
		name  string
		batch []event
		want  error
	}{
		{"negative", []event{{"key", -1}}, frequentitems.ErrNegativeWeight},
		{"overflow", []event{{"key", math.MaxInt64}, {"key", 1}}, frequentitems.ErrWeightOverflow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := runWindow(secret, [][]event{tc.batch}, nil, &out); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if out.Len() != 0 {
				t.Fatal("published a failed window")
			}
		})
	}
	var out bytes.Buffer
	if err := runWindow(secret, [][]event{{{"unknown", 42}, {"zero", 0}}}, nil, &out); err != nil {
		t.Fatal(err)
	}
	digest, err := hashKey(secret, "unknown")
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("distinct keys: 2\nhash:%016x tokens=42 bounds=[42,42]\n", digest)
	if out.String() != want || strings.Contains(out.String(), "unknown") {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func testSecret(t *testing.T) sketchhash.Secret {
	t.Helper()
	t.Setenv("LLM_SKETCHKIT_SECRET", "test-only-not-a-deployment-secret-0123456789")
	secret, err := sketchhash.SecretFromEnv("LLM_SKETCHKIT_SECRET")
	if err != nil {
		t.Fatal(err)
	}
	return secret
}

func TestREADMEGatewayRecipe(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```go\\n(.*?)```").FindAllStringSubmatch(string(readme), -1)
	if len(blocks) != 3 {
		t.Fatalf("Go recipe count = %d, want 3", len(blocks))
	}
	program := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(program, []byte(blocks[0][1]), 0o600); err != nil {
		t.Fatal(err)
	}
	secret := testSecret(t)
	command := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "run", program)
	result, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("README Go program: %v\n%s", err, result)
	}
	if string(result) != "estimated distinct prompts: 1\n" {
		t.Fatalf("README Go output: %s", result)
	}
	for _, block := range blocks[1:] {
		if !strings.Contains(strings.Join(strings.Fields(string(source)), " "), strings.Join(strings.Fields(block[1]), " ")) {
			t.Fatal("README gateway excerpt differs from tested source")
		}
	}
	var out bytes.Buffer
	if err := runWindow(secret, [][]event{
		{{"support", 1240}, {"research", 8900}},
		{{"support", 980}, {"coding", 3600}},
	}, []string{"support", "research", "coding"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), out.String()) {
		t.Fatalf("gateway output not documented: %s", out.String())
	}
}
