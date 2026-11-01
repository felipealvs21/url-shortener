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

> **Já tem um `.env` de antes?** Acrescente a ele a linha `DATABASE_URL=...` do `.env.example`: a aplicação não sobe sem ela.

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

A aplicação lê variáveis de ambiente. Se uma variável opcional não estiver definida, vale o padrão. `DATABASE_URL` é obrigatória e não tem padrão. Valores inválidos ou ausentes impedem a aplicação de subir, e todos os problemas são listados juntos.

| Variável | Padrão | Significado |
|---|---|---|
| `HTTP_ADDR` | `127.0.0.1:8080` | Endereço `host:porta` em que o servidor HTTP escuta. A porta deve ser numérica, de 1 a 65535; o host não é verificado na configuração (um host inválido falha ao abrir a porta). O padrão escuta só no loopback; `:8080` expõe o servidor na rede local. |
| `LOG_LEVEL` | `info` | Nível mínimo de log: `debug`, `info`, `warn` ou `error` (sem diferenciar maiúsculas). |
| `SHUTDOWN_TIMEOUT` | `10s` | Prazo para as requisições em andamento terminarem no encerramento (duração positiva, como `10s` ou `1m`). |
| `DATABASE_URL` | nenhum (obrigatória) | URL de conexão do PostgreSQL, por exemplo `postgres://usuario:senha@127.0.0.1:5432/banco`. **É segredo** (contém a senha): a URL e a senha nunca são registradas em log nem aparecem em mensagens de erro (as mensagens de erro de conexão mostram usuário, banco e `host:porta`). Usuário, senha, banco e porta devem bater com os `POSTGRES_*` do `.env`. Veja o formato abaixo. |

A aplicação lê só o ambiente do processo; quem carrega o `.env` é o Makefile.

#### Formato da `DATABASE_URL`

- Só é aceita a forma de URL, `postgres://` ou `postgresql://`, com host, usuário e nome do banco: `postgres://usuario:senha@host:5432/banco`. O formato chave=valor (`host=... user=...`) não é aceito. Use um único host.
- **Caracteres especiais no usuário e na senha precisam ser percent-encoded**: `@` → `%40`, `#` → `%23`, `$` → `%24`, `:` → `%3A`, `/` → `%2F`, `?` → `%3F`, espaço → `%20`. Sem isso a URL pode ser rejeitada na partida (com uma mensagem que não revela a senha) ou, em casos raros, ser interpretada de outro jeito; codifique sempre. Isso vale também para o `.env`: no Makefile, `#` começa um comentário (o resto da linha é cortado) e `$` é variável do Make.
- No `.env`, escreva a URL **sem aspas**.
- Campos que faltarem na URL (senha, `sslmode`...) podem ser completados pelo driver com variáveis `PG*` (`PGPASSWORD`, `PGSSLMODE`...) e pelo `~/.pgpass`: é o comportamento padrão do libpq, que não desligamos.
- Se o banco estiver travado (em vez de recusar a conexão), a subida falha em até 5 segundos.

### Aplicação

A aplicação **não sobe sem o banco**: ela confirma a conexão na partida e, se falhar, encerra na hora (sem abrir a porta HTTP). Por isso, suba o banco antes:

```sh
make db-up
make run
curl localhost:8080/healthz
curl localhost:8080/readyz
```

(`make run` equivale a `go run ./cmd/api` com o `.env` carregado). Para parar, use `Ctrl+C`.

Se o banco estiver parado, o servidor falha em instantes com uma mensagem como esta (o final muda conforme o motivo):

```text
"msg":"server failed","error":"open database: ping database: failed to connect to `user=shortener database=shortener`: 127.0.0.1:5432 (127.0.0.1): dial error: dial tcp 127.0.0.1:5432: connect: connection refused"
```

Os dois endereços respondem coisas diferentes. `/healthz` (liveness) só diz que o processo está de pé e nunca consulta o banco: responde `200` mesmo com o banco fora do ar, pois reiniciar a aplicação não resolveria isso. `/readyz` (readiness) testa o banco: responde `200` se ele responde e `503` se não; quando o banco volta, `/readyz` volta a `200` sozinho, sem reiniciar a aplicação.

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
