<!-- doc-id: spec-dialect -->
<!-- source-sha256: 6b4f9866e1fce917574db4b22e9aeb930985e7bae17fe2cc09ddf24d04365247 -->
# Capability table

[English](60-dialect.md)

orm은 DSN scheme `markdown://`으로 corpus에 접속하며, 이 scheme은 SQL dialect가 아니라 **IR executor**를
고른다(server와 DSN은 [operations](40-ops.md)에 있다). 이 spec은 통합 계약이다: wire, 그리고 각 orm
surface가 corpus에서 무엇을 하는지, 또는 어떤 orm 오류 code로 답하는지 — executor가 실행하지 않는
surface는 `CAPABILITY_UNSUPPORTED`, 그리고 orm의 규칙이 모든 backend에 주는 대로 `OPERATOR_NOT_ALLOWED`,
`IR_INVALID`, `CODEC_ENCODE`. 계약의 orm 쪽 — protocol 절, 오류 code `WRITE_CONFLICT`, executor가
할당하는 column, orm `docs/dialects.md`의 capability table — 은 orm 자신의 작업이며 orm의 checklist에서
추적한다. Go client가 먼저 구현하고, 다른 client는 각자 orm 항목이다.

## Wire

- **요청**: 요청마다 `{ir_version, manifest_hash, request, params, now}`. client는 요청을
  검증(`ir.Validate`)하고 plan하지 않는다. `params`는 runtime value model JSON을 쓴다 — decimal은
  문자열, bytes는 base64, datetime은 UTC `YYYY-MM-DD HH:MM:SS.ffffff`, time은 `HH:MM:SS.ffffff`이며, column의
  정밀도 p 너머의 자리는 값이 갖지 않는 0이다 — 그 자리에 다른 숫자를 가진 parameter는 `datetime(p)`나
  `time(p)`의 값이 아니다(`CODEC_ENCODE`). codec 단계는 SQL과 같이 client가 적용한다. 채우는 slot의
  type — 비교하거나 할당하는 column, column 함수의 결과, `contains`와 `contains_binary`의 `text`, 상대
  시각 함수의 `i32`나 `f64` 간격 — 의 값이 아닌 parameter는 어떤 행도 읽기 전에 `CODEC_ENCODE`로 요청을
  실패시키며, parameter index와 type을 밝힌다: JSON 종류, type의 범위, decimal의 scale과 precision,
  time이나 datetime의 정밀도, `varchar`의 길이, 텍스트의 U+0000, uuid의 소문자 정준 형태를 검사하며, 모든
  orm client가 bind 값을 검사하는 것과 같다
  ([orm protocol](https://github.com/polyspec/orm/blob/main/docs/protocol.md)). NULL은 모든 slot의
  값이지만, non-null column의 할당에서는 `CODEC_ENCODE`다. 근거: slot 밖의 bind 값에 대한 orm의 code는
  `CODEC_ENCODE`이며, 읽기 전의 검사는 bind 실패처럼 답을 행과 무관하게 한다. face 작업의 값은 엔진 자신의 쓰기이므로 이렇게 검사하지 않으며, catalog의 규칙이 판정하고 `CONSTRAINT`로 거절한다([constraints](50-constraints.md)). `IR_INVALID`는 구조의
  code이고, 이 요청은 형태가 올바르다.
- **읽기의 답**: `one`, `all`, `group_count`는 `{"rows": [...]}`, `count`는 `{"count": n}`, `sum`과
  `avg`는 `{"value": v}`, `paginate`는 `{"rows": [...], "count": n}`이며 `n`은 일치하는 모든 행을 센다.
  행은 JSON object다: projection 순서의 출력, 그다음 각 join(중첩 object, 또는 일치가 없는 left join이면
  null)과 각 relation(`one` relation은 object나 null, `many` relation은 array, 또는 `key_by`로 key를 단
  object). `flatten` relation은 `one` relation으로 답하고 client model이 병합한다
  ([orm protocol](https://github.com/polyspec/orm/blob/main/docs/protocol.md)). 값은 parameter의 value
  model을 쓴다: datetime은 `YYYY-MM-DD HH:MM:SS.ffffff`, time은 `HH:MM:SS.ffffff`, decimal과 bytes는
  문자열, 정수나 `f64`는 JSON number, `bool`은 JSON bool, date와 uuid는 읽을 수 있는 텍스트. 답이 그
  형태로 쓸 수 없는 저장 값은 `INTERNAL`이며, 저장된 텍스트로 답하지 않는다.
- **쓰기의 답**: `{affected, keys}`. `keys`는 insert된 각 행의 생성되거나 주어진 primary key를 insert
  순서로 담으며, `on_duplicate` update이면 만난 행의 key다. `affected`는 insert되거나 update된 각 행을
  한 번 센다 — `on_duplicate`가 있는 insert는 1이다 — 그리고 update는 일치한 행을 센다. 근거: orm의
  client는 모든 database에서 update의 일치한 행을 세며, update하는 upsert는 orm이 1로 맞추기 전까지
  MySQL에서 2, PostgreSQL과 SQLite에서 1이다.
- **오류**: orm `errors.yaml`의 code 하나이며 `{"error": {"code", "message"}}`로 쓴다. hash 불일치는
  `SCHEMA_HASH_MISMATCH`다.
- **control message**: `{op: begin | commit | rollback | savepoint | release | rollback_to, name?}`.
  `begin`은 boolean `read_only`도 받는다. 각각 `{"ok": true}`나 오류로 답한다. 연결은 session이며 그 첫
  message는 DSN의 corpus와 token을 담은 `{"op": "connect", "corpus", "token"}`이다
  ([operations](40-ops.md#the-server)).

## Surface

| Surface | 판정 |
| --- | --- |
| `one`, `all`, `count`, `group_count`, `sum`, `avg` | memory의 corpus 위에서 지원. `group_count`는 NULL을 함께 묶는다 |
| 정렬 | IR `order`는 column의 type으로 정렬하며, NULL은 오름차순에서 모든 값 앞에, 내림차순에서 모든 값 뒤에 오고, 동률은 저장 순서다 — `group_count`의 group과 `limit_per_parent`의 순위에도 같다. 없는 읽기는 저장 순서를 돌려준다([document model](20-document-model.ko.md#순서)). 근거: MySQL과 SQLite는 오름차순에서 NULL을 처음에, PostgreSQL은 마지막에 둔다. MySQL에는 `NULLS FIRST`가 없으므로, NULL을 가장 작게 두는 것이 세 database가 흉내 없이 render하는 유일한 규칙이다 |
| `group_by` | `count`와 `group_count`의 root에서만 유효하며, 그 밖에서는 `IR_INVALID`다. 그것이 있으면 `order` 항목은 group column, 그 column 함수, 또는 `random`을 가리킨다(아니면 `IR_INVALID`) — orm의 규칙. 근거: 모든 행 읽기는 primary key를 고르므로, group된 행 읽기는 MySQL 1055, PostgreSQL 42803, SQLite에서는 임의의 행이며, group되지 않은 column으로의 정렬도 같다 |
| `paginate` | 지원. 요청 하나가 일관된 corpus 하나를 읽는다 |
| `insert`(단일, 다중 행) | 지원. `identity`, `key_prefix`, `order` column은 executor가 할당하며 생략할 수 있다. default는 명시적으로 쓴다. insert는 값이나 NULL로 배정한다 — `plus_p`와 `minus_p`는 거기서 `IR_INVALID`이고 `on_duplicate`에서는 유효하다(orm의 규칙). 근거: MySQL은 그런 배정을 default에서 계산하고, PostgreSQL과 SQLite는 거절한다 |
| `update` | 지원. 읽은 뒤 바뀐 대상은 덮어쓰지 않고 `WRITE_CONFLICT`로 실패한다. primary key나 identity column의 배정은 `IR_INVALID`다([document model](20-document-model.ko.md#key)) |
| `delete`, `restore` | 지원. `restore`는 SQL 형태를 지킨다(soft delete column을 비우고, 지운 동안 unique key를 유지). 참조되는 행의 delete는 같은 delete가 참조하는 행을 지우더라도 `FOREIGN_KEY`([constraints](50-constraints.ko.md#key와-참조)). primary key, identity, soft delete column을 배정하는 `restore`는 `IR_INVALID`다 |
| `optimistic` | 지원. 일치가 없으면 `OPTIMISTIC_LOCK` |
| `on_duplicate` | insert된 행과 만날 수 있는 key — insert가 `identity`나 `key_prefix`에 맡기지 않는 한 primary key, 그리고 각 unique key — 가 정확히 하나이고 insert가 그 모든 column을 배정할 때 지원한다. 그 key가 충돌 대상이며, 그 밖의 `on_duplicate`는 `IR_INVALID`다(MySQL의 아무 key 규칙과 PostgreSQL과 SQLite의 충돌 대상을 같은 key로 만드는 orm의 규칙). primary key는 아래 비교가 정하는 대로 행과 만난다. 근거: MySQL은 자신의 key 순서로 아무 unique key에서나 행과 만나는 반면, PostgreSQL과 SQLite는 충돌 대상 하나를 이름 붙이고 다른 key에서의 만남을 `DUPLICATE_KEY`로 답한다. 행과 만날 수 있는 key가 하나이면 셋은 구성상 같다 |
| primary key 비교 | A–Z의 대소문자를 가리지 않는다. SQL에서 orm의 binary collation과 다른데, key는 file 이름이자 anchor이기 때문이다([document model](20-document-model.ko.md#key)): key가 다른 key와 대소문자를 무시하고 같은 insert는 `DUPLICATE_KEY`이며, 충돌 key가 primary key인 `on_duplicate` insert는 그 행 — 바이트 단위로 같은 key가 먼저, 아니면 byte 순서에서 가장 작은 것 — 과 만나고, 그 행은 자기 key를 유지한다. 근거: `ON DUPLICATE KEY UPDATE`와 `ON CONFLICT`는 충돌 key의 index가 그 index의 비교로 보고하는 중복에서 작동한다 |
| `lock` `update`, `share` | 문서화된 no-op으로 받는다: server transaction이 이미 corpus lock을 잡고 있다 |
| `lock` `update_nowait`, `share_nowait` | `CAPABILITY_UNSUPPORTED` |
| relation(`limit_per_parent`, `if_parent`, `flatten`, `key_by`, `no_cascade_delete`) | 지원. executor가 조립하며, `flatten` relation은 `one` relation으로 답하고 client model이 그것을 부모의 array와 JSON 형태에 병합한다(orm의 규칙). `key_by`는 non-null column을 가리킨다(아니면 `IR_INVALID`). 근거: orm은 병합을 client model의 array와 JSON 형태에 두며, 부모가 가진 이름은 부모의 값을 유지한다. 병합된 답은 client에게 부모 자신의 column과 구별할 수 없는 행을 넘긴다. NULL collection key는 client 언어마다 다르게 답하므로, nullable `key_by`는 형태로 거절한다 |
| subquery(`tuple_in`, `tuple_not_in`, scalar `Sub` aggregate) | 지원 |
| scalar column subquery(`agg` 없는 `columns.sub`) | `limit` count 1과, 그 entity의 모든 primary key column을 담고 `random`이 없는 `order`가 있을 때 유효하며, 아니면 `IR_INVALID`다(orm의 규칙). 값은 그 행의 column이며, 일치하는 행이 없으면 NULL이다. 근거: 여러 행의 scalar subquery는 MySQL 1242, PostgreSQL 21000, SQLite에서는 첫 행이므로, 한 행을 내는 형태만이 하나의 결과를 가진다 |
| column 비교(`*_col`, subquery의 `in`, join `left`/`right`, relation key) | 두 column은 dbspec의 check operand처럼 만난다 — 정수끼리, scale이 같은 `decimal`, `f64`, `varchar`와 `text`, 정밀도가 같은 `time`이나 `datetime`, 그리고 `uuid`, `date`, `bool`, `bytes`는 각각 자신과. 다른 쌍은 `OPERATOR_NOT_ALLOWED`다(orm의 규칙). 근거: PostgreSQL은 `integer = varchar`를 거절하고, MySQL은 변환하며, SQLite는 affinity로 비교한다. `date = datetime`, scale이 다른 decimal, 2^53 근처의 정수와 `f64`도 세 database에서 갈린다 |
| `contains`(`Lk` 연산자) | 지원: column의 텍스트가 parameter의 텍스트를 substring으로 담으면 참이다. ASCII 문자 A–Z와 a–z는 대소문자를 가리지 않고, 그 밖의 문자는 code point로 비교한다. NULL column이나 parameter는 unknown이며, parameter의 `%`, `_`, `\`는 문자 그대로다. 근거: SQL parity — orm의 `COLLATE "C"` 아래 PostgreSQL `ILIKE`, SQLite `LIKE`, orm의 MySQL ASCII fold가 이 결과를 준다 |
| `contains_binary`(`Lb` 연산자) | 지원: column의 텍스트가 parameter의 텍스트를 substring으로 담으면 참이며, code point로 비교한다. NULL column이나 parameter는 unknown이며, parameter의 `%`, `_`, `\`는 문자 그대로다. 근거: `utf8mb4_0900_bin` 아래 MySQL `LIKE`, `COLLATE "C"` 아래 PostgreSQL `LIKE`, SQLite `instr`가 일치한다 |
| `now`, `default now`, `updated`, `soft_delete` | 요청마다 값 하나인 요청의 `now`: `datetime(p)`에 쓸 때는 p자리로 자르고, 비교할 때는 여섯 자리다([operations](40-ops.ko.md#시계)). 결과가 0001-01-01부터 9999-12-31 23:59:59.999999 밖에 있거나 간격을 표현할 수 없는 상대 시각 함수는 NULL이다(orm의 규칙). 근거: MySQL과 SQLite는 범위를 넘으면 NULL을 주며, NULL은 다른 dialect를 guard로 맞출 수 있는 유일한 결과다 |
| `soft_delete` | 모든 읽기는 soft delete된 행을 제외한다 — root, 각 join 자식(join 조건에서 제외하므로 left join은 null을 묶는다), 각 relation과 각 subquery. `restore`만이 그런 행에 일치한다. 근거: 읽기는 늘 soft delete된 행을 제외한다는 orm의 규칙 |
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
| `fulltext` | `OPERATOR_NOT_ALLOWED`: SQLite는 table에 full-text index가 없다(orm dialects.md) |
| lock, local setting, privilege, AES rotation utility | `CAPABILITY_UNSUPPORTED` |
| schema install, dbplan chain | `CAPABILITY_UNSUPPORTED`. `flowmark install`이 corpus를 만들고([schema](30-schema.md)), 진화는 [migration](80-migration.md)을 따른다 |
| 사용할 수 없는 table(`format.block`, `format.section`, `format.encoding`이나 그 행 사이의 `parent.cycle`이 있는 file, 바이트 단위로 같은 key를 가진 두 행, 또는 자리의 `corpus.walk` 항목) | 그 행이 필요한 모든 요청은 읽기든 쓰기든 file과 진단을 밝히며 `CONSTRAINT`로 실패하며, 다른 file들의 행만으로 답하지 않는다([canonical format](10-canonical-format.ko.md#진단)) |

이 표의 conformance 계약은 [conformance](90-conformance.md)다: corpus 전, 요청, 기대하는 corpus 후나 행.
