package aima

import "testing"

func TestIsValidacaoURL(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://portal-renovacoes.aima.gov.pt/ords/r/aima/aima-pr/validar?p71_lang=pt", true},
		{"https://portal-renovacoes.aima.gov.pt/ords/r/aima/aima-pr/validar/12345", true},
		{"https://Portal-Renovacoes.AIMA.gov.pt/ords/r/aima/aima-pr/validar", true},
		{"https://portal-renovacoes.aima.gov.pt/ords/r/aima/aima-pr/cidadao", false},
		{"https://example.com/ords/r/aima/aima-pr/validar", false},
		{"ftp://portal-renovacoes.aima.gov.pt/ords/r/aima/aima-pr/validar", false},
		{"not a url", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsValidacaoURL(c.url); got != c.want {
			t.Errorf("IsValidacaoURL(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestNormalizeURL_DeterministicAcrossOrder(t *testing.T) {
	a := "https://portal-renovacoes.aima.gov.pt/ords/r/aima/aima-pr/validar?p71_lang=pt&token=abc"
	b := "https://portal-renovacoes.aima.gov.pt/ords/r/aima/aima-pr/validar?token=abc&p71_lang=pt"
	canA, hashA, err := NormalizeURL(a)
	if err != nil {
		t.Fatal(err)
	}
	canB, hashB, err := NormalizeURL(b)
	if err != nil {
		t.Fatal(err)
	}
	if canA != canB {
		t.Errorf("canonical mismatch:\n%s\n%s", canA, canB)
	}
	if hashA != hashB {
		t.Errorf("hash mismatch: %s vs %s", hashA, hashB)
	}
}

func TestNormalizeURL_StripsFragmentAndLowercasesHost(t *testing.T) {
	in := "HTTPS://Portal-Renovacoes.AIMA.gov.pt/ords/r/aima/aima-pr/validar?token=abc#section"
	can, _, err := NormalizeURL(in)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://portal-renovacoes.aima.gov.pt/ords/r/aima/aima-pr/validar?token=abc"
	if can != want {
		t.Errorf("got %q, want %q", can, want)
	}
}

func TestNormalizeURL_DifferentURLsHaveDifferentHashes(t *testing.T) {
	_, h1, _ := NormalizeURL("https://portal-renovacoes.aima.gov.pt/ords/r/aima/aima-pr/validar?token=A")
	_, h2, _ := NormalizeURL("https://portal-renovacoes.aima.gov.pt/ords/r/aima/aima-pr/validar?token=B")
	if h1 == h2 {
		t.Fatal("different URLs got identical hashes")
	}
}

func TestNormalizeURL_RejectsInvalid(t *testing.T) {
	if _, _, err := NormalizeURL("just-a-string"); err == nil {
		t.Fatal("expected error for URL without scheme/host")
	}
}
