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

// Порядок query-параметров НЕ нормализуется: APEX привязывает checksum
// (cs=...) к конкретному порядку. Один и тот же токен с переставленными
// параметрами — это другой URL с другим hash.
func TestNormalizeURL_PreservesParamOrder(t *testing.T) {
	a := "https://portal-renovacoes.aima.gov.pt/ords/r/aima/aima-pr/validar?p71_lang=pt&p72_token=abc&cs=XYZ"
	b := "https://portal-renovacoes.aima.gov.pt/ords/r/aima/aima-pr/validar?p72_token=abc&p71_lang=pt&cs=XYZ"
	canA, hashA, err := NormalizeURL(a)
	if err != nil {
		t.Fatal(err)
	}
	canB, hashB, err := NormalizeURL(b)
	if err != nil {
		t.Fatal(err)
	}
	// Разный порядок → разные canonical и hash (cs привязан к порядку).
	if canA == canB {
		t.Errorf("expected different canonical for different param order, got same: %s", canA)
	}
	if hashA == hashB {
		t.Errorf("expected different hash for different param order, got same")
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
