# ADR-0001: Monólito modular evolutivo

- **Status:** aceita
- **Data:** 2026-10-31
- **Decisores:** Felipe

## Contexto

O sistema tem cinco áreas com responsabilidades distintas: Identity, URL Management, Redirect Engine, Analytics e Geolocation. Elas têm perfis de carga muito diferentes. O redirecionamento é muito lido e precisa de baixa latência; o analytics recebe muitas escritas e tolera atraso. É tentador separá-las em microsserviços desde o início.

O projeto, porém, tem um desenvolvedor, nenhum tráfego real e o objetivo de aprender. Microsserviços trazem custos que só se pagam com escala e com equipes independentes: rede entre serviços, deploy e observabilidade distribuídos, consistência eventual e versionamento de contratos.

## Decisão

Um único binário e um único deploy, organizado em **módulos com fronteiras explícitas**:

- cada módulo vive em `internal/<modulo>/` e expõe uma API pequena para os outros;
- um módulo não acessa as tabelas de outro diretamente;
- a comunicação entre módulos acontece por chamadas de função no processo e, quando fizer sentido, por eventos (por exemplo, o redirect publicando cliques para o analytics).

Um módulo só é extraído para um serviço quando houver uma **justificativa medida**, como uma carga que precisa escalar de forma independente, e essa extração exige uma nova ADR.

## Alternativas consideradas

- **Microsserviços desde o início:** rejeitada. Custo operacional alto sem benefício nesta escala, e o aprendizado de Go ficaria em segundo plano diante da infraestrutura.
- **Monólito em camadas sem módulos** (handlers/services/repositories globais): rejeitada. Fica simples no começo, mas as fronteiras entre domínios se perdem e uma extração futura vira reescrita.

## Consequências

- Positivas: deploy, testes e depuração simples. Transações locais no PostgreSQL. As fronteiras ficam visíveis no código e preparam uma extração futura.
- Negativas: as fronteiras dependem de disciplina, porque o compilador só impõe `internal/` em relação a módulos Go externos, não entre pacotes do próprio projeto. Todos os módulos escalam juntos até haver uma extração.
- A revisar quando: as medições com k6 mostrarem que o redirect e o analytics precisam escalar de forma diferente.
