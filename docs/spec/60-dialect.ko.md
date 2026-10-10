<!-- doc-id: spec-dialect -->
<!-- source-sha256: b2d123666f16187ae8d0035a2779dc8eeb65553be76e5c3e026d1926b0045fff -->
# Capability table

[English](60-dialect.md)

orm은 DSN scheme `markdown://`으로 corpus에 접속하며, 이 scheme은 SQL dialect가 아니라 **IR executor**를
고른다(server와 DSN은 [operations](40-ops.md)에 있다). 이 spec은 통합 계약이다: wire, 그리고 각 orm
surface가 corpus에서 무엇을 하는지 또는 orm 전통에 따라 `CAPABILITY_UNSUPPORTED`나
`OPERATOR_NOT_ALLOWED`로 거절하는지. 계약의 orm 쪽 — protocol 절, 오류 code `WRITE_CONFLICT`, executor가
할당하는 column, orm `docs/dialects.md`의 capability table — 은 orm 자신의 작업이며 orm의 checklist에서
추적한다. Go client가 먼저 구현하고, 다른 client는 각자 orm 항목이다.

## Wire

- **요청**: 요청마다 `{ir_version, manifest_hash, request, params, now}`. client는 요청을
  검증(`ir.Validate`)하고 plan하지 않는다. `params`는 runtime value model JSON을 쓴다 — decimal은
  문자열, bytes는 base64, datetime은 UTC `YYYY-MM-DD HH:MM:SS.ffffff`. codec 단계는 SQL과 같이 client가
  적용한다.
- **읽기의 답**: 요청의 형태 — join, relation, `key_by`, `flatten` — 로 조립된 행.
- **쓰기의 답**: `{affected, keys}`. `keys`는 insert된 각 행의 생성되거나 주어진 primary key를 insert
  순서로 담는다.
- **오류**: orm `errors.yaml`의 code 하나. hash 불일치는 `SCHEMA_HASH_MISMATCH`다.
- **control message**: `{op: begin | commit | rollback | savepoint | release | rollback_to, name?}`.
  각각 `{ok}`나 오류로 답한다. 연결은 session이다.

## Surface

| Surface | 판정 |
| --- | --- |
| `one`, `all`, `count`, `group_count`, `sum`, `avg` | memory의 corpus 위에서 지원. `group_count`는 NULL을 함께 묶는다 |
| 정렬 | IR `order`는 column의 type으로 정렬한다. 없는 읽기는 저장 순서를 돌려준다([document model](20-document-model.md)) |
| `paginate` | 지원. 요청 하나가 일관된 corpus 하나를 읽는다 |
| `insert`(단일, 다중 행) | 지원. `identity`, `key_prefix`, `order` column은 executor가 할당하며 생략할 수 있다. default는 명시적으로 쓴다 |
| `update` | 지원. 읽은 뒤 바뀐 대상은 덮어쓰지 않고 `WRITE_CONFLICT`로 실패한다 |
| `delete`, `restore` | 지원. `restore`는 SQL 형태를 지킨다(soft delete column을 비우고, 지운 동안 unique key를 유지). 참조되는 행의 delete는 `FOREIGN_KEY` |
| `optimistic` | 지원. 일치가 없으면 `OPTIMISTIC_LOCK` |
| `on_duplicate` | 충돌 대상 **모든 unique key**(MySQL 규칙)로 지원. unique key가 여럿인 table에서는 PostgreSQL과 SQLite의 대상 규칙과 다르며 그 차이를 여기 밝힌다 |
| `lock` `update`, `share` | 문서화된 no-op으로 받는다: server transaction이 이미 corpus lock을 잡고 있다 |
| `lock` `update_nowait`, `share_nowait` | `CAPABILITY_UNSUPPORTED` |
| relation(`limit_per_parent`, `if_parent`, `flatten`, `key_by`, `no_cascade_delete`) | 지원. executor가 조립한다 |
| subquery(`tuple_in`, `tuple_not_in`, scalar `Sub` aggregate) | 지원 |
| `now`, `default now`, `updated`, `soft_delete` | orm 시계 규칙에 따른 요청의 `now` |
| codec columns | orm 요청에서는 client가 적용한다. 엔진이 시작하는 쓰기(CLI, MCP, sidecar, 이력)는 orm의 Go codec package로 codec을 적용하며, 검증은 같은 package로 decode한다. 저장 값은 encode된 값이며, byte를 만드는 단계는 base64다 |
| indexes | 허용되며 메모리에서 유도된다 |
| `select explicit`, `entity`, `navigation` | client 쪽이며 저장에 효과가 없다 |
| `aes`, `aes_version`, `blind_index` | corpus에서 schema 오류 — key는 client를 떠나지 않는다 |
| `audit`, `immutable` setting | corpus에서 schema 오류. 대신 `state_machine … history` |
| transaction | session 위의 `begin`부터 `commit`/`rollback`까지. `readOnly`는 지키며 lock을 잡는다 |
| savepoint | 중첩 stage |
| `orderByRandom` | 지원. 순서에 seed를 두지 않는다 |
| audit 값을 가진 transaction | `CAPABILITY_UNSUPPORTED` |
| `isolation`, `timeoutMs` option | `CAPABILITY_UNSUPPORTED` |
| `getQuery()`, statement event, `subscribe()` | `CAPABILITY_UNSUPPORTED` — SQL 텍스트가 없다 |
| `force_index` | `CAPABILITY_UNSUPPORTED` — index는 이름 붙은 것이 아니라 유도된다 |
| `fulltext`, `Lb` 연산자 | `OPERATOR_NOT_ALLOWED` |
| lock, local setting, privilege, AES rotation utility | `CAPABILITY_UNSUPPORTED` |
| schema install, dbplan chain | `CAPABILITY_UNSUPPORTED`. `flowmark install`이 corpus를 만들고([schema](30-schema.md)), 진화는 [migration](80-migration.md)을 따른다 |

이 표의 conformance 계약은 [conformance](90-conformance.md)다: corpus 전, 요청, 기대하는 corpus 후나 행.
