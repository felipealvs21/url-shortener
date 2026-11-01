# ADR-0002: `net/http` padrão antes do Chi

- **Status:** aceita
- **Data:** 2026-10-31
- **Decisores:** Felipe

## Contexto

A stack planejada inclui o roteador Chi. Desde o Go 1.22, o `http.ServeMux` da biblioteca padrão aceita método e curingas no padrão (`GET /readyz`, `GET /{code}`), o que antes era o principal motivo para usar um roteador externo. Na Etapa 1 só existem dois endpoints de health check.

## Decisão

Começar com `net/http` puro. Adotar o Chi quando surgir um problema que ele resolve melhor que a biblioteca padrão, sobretudo:

- grupos de rotas com middlewares diferentes (por exemplo, autenticação só no gerenciamento);
- montar sub-roteadores por módulo (`r.Mount("/api/urls", ...)`);
- uma cadeia de middlewares que fique difícil de ler com composição manual.

A adoção será registrada em uma ADR que substitui esta.

## Alternativas consideradas

- **Chi desde o início:** dependência sem uso real agora, e não deixaria claro o que ele acrescenta.
- **Gin/Echo:** frameworks com contexto próprio, fora do padrão `http.Handler`. O Chi é compatível com `net/http`, então trocar depois custa pouco.

## Consequências

- Nenhuma dependência de roteamento na Etapa 1. Os handlers são `http.Handler` puros, que o Chi também aceita, então a migração futura muda só o registro das rotas.
- Atenção: os padrões novos do ServeMux dependem da linha `go` do `go.mod` ser 1.22 ou mais.
