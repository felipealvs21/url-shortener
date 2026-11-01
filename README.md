# URL Shortener

Encurtador de URLs com analytics, escrito em Go. É um projeto de portfólio, construído como um monólito modular evolutivo, com foco em clareza do código e em decisões arquiteturais justificadas.

Status: Etapa 1 — Fundação (em andamento)

## Documentação

- [Decisões de arquitetura (ADRs)](docs/adr/)

## Desenvolvimento local

### Pré-requisitos

- Go, na versão indicada na linha `go` do `go.mod`.
- Docker com Compose v2 ou superior (`docker compose version`).

### Configuração

```sh
cp .env.example .env
```

O `.env` guarda a configuração local do banco e nunca é versionado. Os valores do `.env.example` servem só para desenvolvimento local.

### Banco de dados (PostgreSQL)

Subir o banco em segundo plano e conferir que está saudável (`healthy`):

```sh
docker compose up -d
docker compose ps
```

Abrir um `psql` dentro do container:

```sh
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
```

As aspas simples fazem as variáveis serem expandidas dentro do container, com os valores do seu `.env`.

Parar o banco, mantendo os dados:

```sh
docker compose down
```

> **Atenção, destrutivo:** o comando abaixo apaga o volume e **todos os dados do banco local**. Não há como desfazer.
>
> ```sh
> docker compose down -v
> ```

O banco escuta apenas em `127.0.0.1` (porta `5432` por padrão, configurável com `POSTGRES_PORT` no `.env`).

### Aplicação

```sh
go run ./cmd/api
curl localhost:8080/healthz
```

### Testes

```sh
go test ./...
```
