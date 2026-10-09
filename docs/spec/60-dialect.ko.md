<!-- doc-id: spec-dialect -->
<!-- source-sha256: f777cd57df769aae9e8ae78b32b8c1e49c0be8124ac9e724921498d339170c79 -->
# Capability table

[English](60-dialect.md)

markdown backend는 DSN scheme `markdown://<path>`로 선택되는 네 번째 **executor**다. shared
planner가 SQL backend와 똑같이 IR request를 검증하고, executor는 request를 소비하며 SQL을
render하지 않는다. 이 table이 통합 계약이다: 각 표면이 무엇을 지원하고 무엇을
`CAPABILITY_UNSUPPORTED`·`OPERATOR_NOT_ALLOWED`로 거부하는지 — orm 전통대로. 지원은 다른 client가
executor를 port하기 전까지 **Go-only**다.

| 표면 | 판정 |
| --- | --- |
| `one`, `all`, `count`, `group_count`, `sum`, `avg` | in-memory corpus 위에서 지원; ordering은 type-directed, `group_count`는 NULL을 같은 group으로 처리한다 |
| `paginate` | 지원; session이 corpus revision을 가지고 다니며 불일치 시 `CORPUS_CHANGED`로 실패하거나 keyset pagination으로 강등한다 |
| `insert` (단일·다중 row) | 하나의 transaction 안에서 지원; primary key 생략은 error이다 (markdown corpus에 `identity` 없음) |
| `update` | 지원되고 **강화된다**: 모든 commit이 lock 아래 재독기하므로, 동시의 사람 편집은 덮어써지는 대신 `CORPUS_CONFLICT`로 실패한다 |
| `delete`, `restore` | 지원; `restore`는 SQL 형식을 유지한다 (soft-delete column 해제, 삭제 중에도 unique key 유지) |
| `optimistic` | 지원 — 평범한 update도 위 강화로 이미 optimistic이고, 명시적 형식은 구분된다 (`OPTIMISTIC_LOCK` vs `NO_ROWS`) |
| `on_duplicate` | 충돌 대상을 **임의의 unique key**로 지원 (MySQL 규칙); unique key가 여러 개인 table에서 PostgreSQL·SQLite의 대상 규칙과 갈라지며 그 차이를 여기에 문서화한다 |
| `lock` (`update`, `share`, `update_nowait`, `share_nowait`) | `update`·`share`는 문서화된 no-op로 받는다 (하나의 작업 단위 lock이 이미 직렬화); `*_nowait`는 `CAPABILITY_UNSUPPORTED`로 거부 |
| relation (`limit_per_parent`, `if_parent`, `flatten`, `key_by`, 복합 key, `no_cascade_delete`) | 지원 — 유도된 index로 corpus 위의 별도 statement |
| subquery (`tuple_in`, `tuple_not_in`, scalar `Sub` aggregate) | 지원 |
| `now`, `default now` | 지원; UTC client clock — orm clock 규칙 그대로 |
| `soft_delete` setting | 지원; soft-delete된 row는 key를 차지하고 foreign key를 만족한다 |
| `audit` setting | advisory — validate 시점 거부뿐 (사람 write에 대한 trigger 시점 예방 없음) |
| codec column (AES, styled value, `blind_index`) | 지원; 검증과 codec 연산은 key를 요구한다 |
| savepoint | 하나의 transaction 안의 nested staging |
| `orderByRandom` | 지원, corpus revision별 seed |
| `getQuery()`, statement event, `subscribe()` | `CAPABILITY_UNSUPPORTED` — 보고할 SQL text가 없다 |
| `force_index` | `CAPABILITY_UNSUPPORTED` — index는 유도되지 명명되지 않는다 |
| `fulltext`, `Lb` 연산자 | `OPERATOR_NOT_ALLOWED` |
| `isolation`, `timeoutMs` connection option | `CAPABILITY_UNSUPPORTED` |
| dbplan chain (`plans.md` lifecycle) | v1에서 거부 — [migration](80-migration.ko.md) 참조; corpus 생성과 additive column 추가뿐 |

이 table의 conformance 계약은 [conformance](90-conformance.ko.md)다: corpus before, request, 기대
corpus after — SQL도 binds도 없다.
