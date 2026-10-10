<!-- doc-id: spec-ops -->
<!-- source-sha256: 29298ffd3a587254b17637c6ecd9f95b5b3f9465ed90ed8a6f5ebd1ae0f65a46 -->
# Operations

[English](40-ops.md)

이 spec은 운영 의미를 고정한다: corpus 열기, server와 그 session, lock, transaction과 commit protocol,
시계, 실패 어휘. commit protocol의 순서는 규범이다.

## Corpus 열기

- 열기는 **복구를 먼저** 실행한다: prepare record가 있는 stage는 완료하고, 없는 stage는 버리며, 현재
  hash가 기록된 어느 hash와도 맞지 않는 대상 — crash 도중 들어온 사람의 편집 — 은 거절하고 보고하며
  조용히 덮어쓰지 않는다.
- 열기는 catalog를 읽고 corpus를 walk하며 문서를 parse하고 index를 유도하고, 바뀌지 않은 file에는
  file content hash를 key로 하는 parse된 행 cache를 다시 쓴다.

## canon 연산

- `flowmark canon <corpus> [file…]`는 지정한 file을(file을 지정하지 않으면 corpus의 모든 문서를) 한 transaction에서 canonical 형식으로 다시 쓴다. 대상은 `format.*` 진단이 있는 file의 commit 거절에서 면제된다. 이름을 준 file에 `format.block`, `format.section`, `format.encoding`이나 그 행 사이의 `parent.cycle`이 있으면 명령은 `CONSTRAINT`로 거절하며 file을 밝히고 아무것도 쓰지 않는다. file을 주지 않으면 대상은 그 행 사이에 `parent.cycle`이 없고 `format.*` 진단이 `format.noncanonical`뿐인 corpus 문서이며, 그 밖의 file은 보고하고 다시 쓰지 않는다. `corpus.walk` file은 결코 대상이 아니다. canon은 논리 값을 바꾸지 않으므로 key, 참조, 제약 검사가 그것을 거절하지 않는다. 링크 유도에 사용할 수 없는 table이 필요한 대상(`files` row로의 링크는 그 key에서만 유도되어 table이 필요 없고, block이나 document row로의 링크는 그 row의 table이 필요하다)은 이름을 준 canon을 `CONSTRAINT`로 거절하며 두 file을 밝히고, file을 주지 않으면 보고만 하고 다시 쓰지 않는다. validator 진단이 있는 값은 쓴 텍스트를 유지한다. 문서 바이트를 정규화하는 유일한 연산이며, 논리적 쓰기는 결코 정규화하지 않는다. 근거: migration은 별도의 명시적 transaction이므로, logical 쓰기는 사용자가 쓰지 않은 바이트를 바꾸지 않는다.

## Server

- `flowmark serve <corpus> --port <port>`는 orm client가 접속하는 상주 face다. `--port`는 필수다.
  loopback TCP에만 bind한다.
  처음 시작할 때 무작위 token을 `.flowmark/server.json`에 쓰며, token은 재시작해도 유지되고
  `flowmark serve --new-token`으로만 바뀐다.
- DSN은 `markdown://127.0.0.1:<port>/<corpus>?token=<token>`이고 `<corpus>`는 corpus root의 directory
  이름이다. corpus나 token 불일치는 `CONFIG`다. 원격 접속과 다른 인증은 v1 밖이다.
- 연결은 session이다: 요청과 control message의 JSON lines이며 각각 순서대로 답한다(요청과 답의 형태는
  [capability table](60-dialect.md)에 있다).

## Lock

- **portable lock 하나.** 모든 operation — 읽기든 쓰기든, 어느 face에서든 — 은 holder 식별자와 stale
  감지를 갖는 같은 lock file을 잡는다: kit holder-lock 선례를 따라 `<pid>@<host>/<session>`을 담은 lock
  file이며 flock이나 OS 전용 primitive는 없다.
- lock은 operation 하나 동안, 또는 server transaction 하나의 `begin`부터 `commit`이나 `rollback`까지
  잡는다. 연결이 닫히거나 transaction이 server flag `tx_idle_ms`보다 오래 쉬면 transaction을
  rollback하고 lock을 푼다.
- CLI, MCP, sidecar는 같은 lock을 기다리며 holder를 밝히는 `LOCK_TIMEOUT`으로 실패한다.

## Transaction

- transaction 안의 요청은 stage된 post-image를 읽는다. savepoint는 중첩 stage이고 `rollback_to`는 그
  위의 stage를 버린다.
- face는 commit이 `CORPUS_CONFLICT`를 만나면 다시 읽은 뒤 단일 operation 쓰기를 한 번 재시도한다.
  server transaction은 server가 재시도하지 않으며 orm client는 `WRITE_CONFLICT`를 받고 이를 재시도하지 않는다.

## Commit protocol

1. 모든 대상이 읽은 뒤 바뀌지 않았는지 확인한다. 바뀌었으면 file과 줄을 밝히는 `CORPUS_CONFLICT`로
   실패한다.
2. post-image를 적용해 lock 아래 다시 읽은 corpus에 대해 제약 검사를 실행한다 — `begin` 때의 snapshot도,
   예측한 대상 file 집합도 아니다. 다른 행의 validator 진단(`type`, `key.duplicate`, `reference.dangling`, `check`, `order`, `state.limit`, `history.gap`, `history.chain`, 여러 `files` file에 걸친 `parent.cycle`)은 commit을 실패시키지 않되, `type` 진단은 commit의 제약 검사가 그 값을 평가해야 할 때 commit을 실패시키고, 바이트 단위로 같은 key를 가진 두 행은 그 table을 사용할 수 없게 한다([canonical format](10-canonical-format.ko.md#진단)); `format.*` 진단은 다시 쓰기 거절과 사용할 수 없는 table 상태([canonical format](10-canonical-format.ko.md#진단))가 정하는 경우에만 commit을 실패시킨다. file을 다시 써야 하는 commit — 행 자신의 file, 상세 섹션이 생기거나 사라지는 행으로의 link를 가진 file, `order`의 형제 — 은 그 file이 `format.*` 진단을 가지면 거절한다: `CONSTRAINT`이며 file과 진단을 밝히고, `format.noncanonical`이면 `flowmark canon`을 밝힌다. 세 commit은 면제된다: 대상에 대한 `flowmark canon`([operations](#canon-연산)), 추가된 column이 일으킨 진단에 대한 add-column migration([migration](80-migration.ko.md)) — 그 진단은 그 migration에 대해 table을 사용할 수 없게 하지 않는다 —, `files` 행의 file 전체를 지우는 delete는 rewrite 거절에서만 면제되며, restrict 검사는 그 행을 참조할 수 있는 모든 table의 행을 여전히 필요로 하며(자신의 table을 `parent`로 하는 것, 자식 및 이력 block, 그 행으로의 foreign key를 가진 모든 table), 그 중 하나라도 사용할 수 없으면 그 delete는 `CONSTRAINT`다.
3. 모든 post-image file을 `.flowmark/tx/<id>/`에 content-addressed로 쓰고 fsync한다.
4. 모든 대상의 pre-image를 stage에 복사하고 fsync한다.
5. **prepare record**를 쓰고 fsync한다: pre-hash와 post-hash를 가진 모든 rename.
6. runtime의 atomic rename으로 rename을 실행한다.
7. commit record를 쓰고 fsync한다.
8. stage를 거둔다.

전원 손실을 넘는 내구성은 이 journal과 열 때의 복구에 기대며, OS 전용 directory fsync 의미에 기대지
않는다.

## 시계

엔진이 할당하는 모든 시각 — `updated`, `soft_delete`, `default now`, 이력 행의 `at` — 은 orm 요청이면
요청의 `now`, 아니면 microsecond로 자른 엔진의 UTC 시계다.

## 비용 모델

- command별 p95 지연 목표를 여기서 선언한다([open questions](110-open-questions.md)의 corpus 규모에서
  수백 millisecond).
- parse된 행 cache는 바뀌지 않은 file의 재parse를 피한다. commit은 transaction이 결과를 바꿀 수 있는
  모든 제약을 검사한다: 대상 행의 key, 참조, check, 순서, limit, 전이, `history.gap`과, 그 행들이 참조하거나
  참조되는 행들이며 — lock 아래 다시 읽은 corpus에서 평가한다. 전체 `flowmark validate`는 요청할 때와
  catalog가 바뀔 때 실행한다.
- 오래 사는 face(sidecar, server)는 corpus를 지켜보다 host에 변경 event를 push할 수 있다. 쓰기는 여전히
  transaction protocol로만 간다.

## 실패 어휘

| face 이름 | orm code | 뜻 |
| --- | --- | --- |
| `LOCK_TIMEOUT` | `CANCELED` | lock 대기가 끝났다. holder를 밝힌다 — SQLite의 `busy_timeout`이 그렇듯 `CANCELED`다 |
| `CORPUS_CONFLICT` | `WRITE_CONFLICT` | 읽은 뒤 대상이 바뀌었다. file과 줄을 밝힌다 |
| `DUPLICATE_KEY` | `DUPLICATE_KEY` | primary나 unique key 값이 이미 있다. 두 자리를 밝힌다 |
| `FOREIGN_KEY` | `FOREIGN_KEY` | 참조가 어떤 행도 가리키지 않거나, 참조된 행을 참조되는 동안 지우려 했다 |
| `CONSTRAINT` | `CONSTRAINT` | catalog의 다른 규칙이 쓰기를 거절했다 — check, 제목, 전이, 필수 column, limit, 순서, 한 줄 자리, 쓰기가 다시 쓸 비정규 또는 잘못된 문서. 규칙과 자리를 밝힌다; 사용할 수 없는 table의 행이 필요한 요청(읽기나 쓰기)도 file과 진단을 밝히며 `CONSTRAINT`다([canonical format](10-canonical-format.ko.md#진단)) |
| 복구 거절 | — | 대상과 기록된 두 hash를 밝힌다. 해결될 때까지 열기를 거절한다 |

모든 실패 메시지는 원인과 고치는 방법을 밝힌다.
