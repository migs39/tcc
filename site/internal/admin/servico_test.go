package admin

import (
	"context"
	"errors"
	"testing"

	"tcc/site/internal/db/dbteste"
)

const senhaBoa = "senha-de-teste-123"

func novoServico(t *testing.T) *Servico {
	t.Helper()
	svc, err := NovoServico(dbteste.Novo(t))
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestCadastrarEAutenticar(t *testing.T) {
	ctx := context.Background()
	svc := novoServico(t)

	a, err := svc.Cadastrar(ctx, "Ana", "ana@exemplo.com", senhaBoa)
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Autenticar(ctx, "ANA@exemplo.com", senhaBoa)
	if err != nil || got.ID != a.ID {
		t.Fatalf("login válido falhou: %v", err)
	}
	if _, err := svc.Autenticar(ctx, "ana@exemplo.com", "senha-errada-123"); !errors.Is(err, ErrCredenciaisInvalidas) {
		t.Fatalf("senha errada: esperado ErrCredenciaisInvalidas, veio %v", err)
	}
	if _, err := svc.Autenticar(ctx, "ninguem@exemplo.com", senhaBoa); !errors.Is(err, ErrCredenciaisInvalidas) {
		t.Fatalf("e-mail inexistente: esperado ErrCredenciaisInvalidas, veio %v", err)
	}
}

func TestCadastrarValida(t *testing.T) {
	ctx := context.Background()
	svc := novoServico(t)

	casos := []struct {
		nome, email, senha string
		erro               error
	}{
		{"", "a@exemplo.com", senhaBoa, ErrNomeVazio},
		{"A", "sem-arroba", senhaBoa, ErrEmailInvalido},
		{"A", "Fulano <a@exemplo.com>", senhaBoa, ErrEmailInvalido},
		{"A", "a@exemplo.com", "curta", ErrSenhaCurta},
	}
	for _, c := range casos {
		if _, err := svc.Cadastrar(ctx, c.nome, c.email, c.senha); !errors.Is(err, c.erro) {
			t.Errorf("Cadastrar(%q, %q): esperado %v, veio %v", c.nome, c.email, c.erro, err)
		}
	}

	if _, err := svc.Cadastrar(ctx, "A", "dup@exemplo.com", senhaBoa); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Cadastrar(ctx, "B", "DUP@exemplo.com", senhaBoa); !errors.Is(err, ErrEmailEmUso) {
		t.Fatalf("e-mail duplicado (maiúsculas): esperado ErrEmailEmUso, veio %v", err)
	}
}

func TestCadastrarPrimeiroSoUmaVez(t *testing.T) {
	ctx := context.Background()
	svc := novoServico(t)
	if _, err := svc.CadastrarPrimeiro(ctx, "Ana", "ana@exemplo.com", senhaBoa); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CadastrarPrimeiro(ctx, "Bia", "bia@exemplo.com", senhaBoa); !errors.Is(err, ErrJaExisteAdmin) {
		t.Fatalf("segundo seed: esperado ErrJaExisteAdmin, veio %v", err)
	}
}

func TestDesativarBloqueiaLoginESessoes(t *testing.T) {
	ctx := context.Background()
	svc := novoServico(t)
	ana, _ := svc.Cadastrar(ctx, "Ana", "ana@exemplo.com", senhaBoa)
	bia, _ := svc.Cadastrar(ctx, "Bia", "bia@exemplo.com", senhaBoa)

	token, _, err := svc.CriarSessao(ctx, bia.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Desativar(ctx, bia.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdminDaSessao(ctx, token); !errors.Is(err, ErrSessaoInvalida) {
		t.Fatalf("sessão de admin desativado continua válida: %v", err)
	}
	if _, err := svc.Autenticar(ctx, "bia@exemplo.com", senhaBoa); !errors.Is(err, ErrCredenciaisInvalidas) {
		t.Fatalf("admin desativado conseguiu entrar: %v", err)
	}

	// Ana é a última ativa.
	if err := svc.Desativar(ctx, ana.ID); !errors.Is(err, ErrUltimoAdminAtivo) {
		t.Fatalf("desativou o último admin ativo: %v", err)
	}

	if err := svc.Reativar(ctx, bia.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Autenticar(ctx, "bia@exemplo.com", senhaBoa); err != nil {
		t.Fatalf("admin reativado não entra: %v", err)
	}
}

func TestTrocarSenhaEncerraSessoes(t *testing.T) {
	ctx := context.Background()
	svc := novoServico(t)
	ana, _ := svc.Cadastrar(ctx, "Ana", "ana@exemplo.com", senhaBoa)
	token, _, _ := svc.CriarSessao(ctx, ana.ID)

	if err := svc.TrocarSenha(ctx, ana.ID, "outra-senha-bem-longa"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdminDaSessao(ctx, token); !errors.Is(err, ErrSessaoInvalida) {
		t.Fatal("sessão sobreviveu à troca de senha")
	}
	if _, err := svc.Autenticar(ctx, "ana@exemplo.com", senhaBoa); err == nil {
		t.Fatal("senha antiga ainda funciona")
	}
	if _, err := svc.Autenticar(ctx, "ana@exemplo.com", "outra-senha-bem-longa"); err != nil {
		t.Fatalf("senha nova não funciona: %v", err)
	}
}

func TestSessaoGuardaSoOHash(t *testing.T) {
	ctx := context.Background()
	pool := dbteste.Novo(t)
	svc, _ := NovoServico(pool)
	ana, _ := svc.Cadastrar(ctx, "Ana", "ana@exemplo.com", senhaBoa)
	token, _, _ := svc.CriarSessao(ctx, ana.ID)

	var achou bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM sessoes_admin WHERE token_hash = convert_to($1, 'UTF8'))`,
		token).Scan(&achou); err != nil {
		t.Fatal(err)
	}
	if achou {
		t.Fatal("token de sessão gravado em claro")
	}
	if _, err := svc.AdminDaSessao(ctx, token); err != nil {
		t.Fatalf("sessão válida rejeitada: %v", err)
	}
	if err := svc.EncerrarSessao(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdminDaSessao(ctx, token); !errors.Is(err, ErrSessaoInvalida) {
		t.Fatal("sessão encerrada continua válida")
	}
}
