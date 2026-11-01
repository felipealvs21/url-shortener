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

O `.env` guarda a configuração local (banco e aplicação) e nunca é versionado. Os valores do `.env.example` servem só para desenvolvimento local.

### Banco de dados (PostgreSQL)

Subir o banco em segundo plano e esperar até que esteja saudável (`healthy`):

```sh
make db-up
docker compose ps
```

(`make db-up` equivale a `docker compose up -d --wait`.)

Abrir um `psql` dentro do container:

```sh
make db-psql
```

(equivale a `docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"'`; as aspas simples fazem as variáveis serem expandidas dentro do container, com os valores do seu `.env`).

Parar o banco, mantendo os dados:

```sh
make db-down
```

(equivale a `docker compose down`).

> **Atenção, destrutivo:** o comando abaixo apaga o volume e **todos os dados do banco local**. Não há como desfazer.
>
> ```sh
> docker compose down -v
> ```

O banco escuta apenas em `127.0.0.1` (porta `5432` por padrão, configurável com `POSTGRES_PORT` no `.env`).

### Configuração da aplicação

A aplicação lê variáveis de ambiente. Se uma variável não estiver definida, vale o padrão (o `.env` local não precisa conter nenhuma delas). Valores inválidos impedem a aplicação de subir, e todos os problemas são listados juntos.

| Variável | Padrão | Significado |
|---|---|---|
| `HTTP_ADDR` | `127.0.0.1:8080` | Endereço `host:porta` em que o servidor HTTP escuta. A porta deve ser numérica, de 1 a 65535; o host não é verificado na configuração (um host inválido falha ao abrir a porta). O padrão escuta só no loopback; `:8080` expõe o servidor na rede local. |
| `LOG_LEVEL` | `info` | Nível mínimo de log: `debug`, `info`, `warn` ou `error` (sem diferenciar maiúsculas). |
| `SHUTDOWN_TIMEOUT` | `10s` | Prazo para as requisições em andamento terminarem no encerramento (duração positiva, como `10s` ou `1m`). |

A aplicação lê só o ambiente do processo; quem carrega o `.env` é o Makefile.

### Aplicação

```sh
make run
curl localhost:8080/healthz
```

(`make run` equivale a `go run ./cmd/api` com o `.env` carregado). Para parar, use `Ctrl+C`.

Ao parar com `Ctrl+C`, o `make` exibe `make: *** [Makefile:...: run] Erro 1`. Isso acontece porque o sinal chega também ao `go run`, que sai com código 1; a aplicação encerrou normalmente se o log mostrar `"server stopped"`.

Para mudar um valor só numa execução, passe-o na linha de comando do `make`:

```sh
make run LOG_LEVEL=debug
```

`LOG_LEVEL=debug make run` **não funciona** quando o `.env` define `LOG_LEVEL`: pela regra de precedência do Make, o valor do `.env` sobrescreve a variável exportada no shell. Variáveis passadas na linha de comando do `make` vencem o `.env`.

### Testes

```sh
make test    # go test -race ./...
make check   # gofmt, go vet e testes; é o alvo padrão (basta rodar `make`)
```
