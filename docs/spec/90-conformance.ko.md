<!-- doc-id: spec-conformance -->
<!-- source-sha256: 9ae4a83b4fa2f5aa3f74cc742e5585175d2b07fd2f256a0c9d27ab026bfd2566 -->
# Conformance

[English](90-conformance.md)

이 문서는 엔진의 conformance 계약과 그 vector의 형식을 적는다.

## 계약

- vector는 **corpus 전 — 그 catalog와 문서 — 과 요청, 그리고 기대하는 corpus 후, 행, 또는 오류**다.
  비교할 SQL 텍스트나 bind 목록은 없다. SQL conformance harness는 적용되지 않으며 이 계약이 그것을
  대신한다.
- 요청은 [capability table](60-dialect.md) 아래의 IR 요청이거나, vector가 face를 시험하는 경우 face
  operation이다. 아래 형식의 vector는 IR 요청을 담는다. face operation vector는 새 엔진 위의 face와
  함께 온다(`T12.10`).
- vector는 flowmark 엔진을 상대로 실행한다: 요청은 [operations](40-ops.md)의 write path — lock, journal,
  그리고 check를 동반한 commit — 를 transaction 밖의 요청으로 지나가므로, vector의 답은 client가 받는
  답이다. orm의 `markdown` IR executor conformance는 orm에 고정한 flowmark build를 상대로 실행되며 orm
  자신의 계약이다.
- 모든 vector는 이름을 밝히며, 실행할 수 없는 run은 orm conformance 전통에 따라 실패다.

## 요구 성질

- **Round trip**: 모든 vector의 corpus 전은 모든 저장에서 canonical이고(`emit(parse(s)) == s`)
  유효하다. 단 vector가 diagnostic과 함께 선언한 file은 예외다. corpus 후는 구성상 canonical이며,
  요청이 바꾸지 않은 file의 선언된 diagnostic 외에는 diagnostic을 갖지 않는다. runner는 corpus를
  validate해 둘 다 확인한다. 이 예외가 있는 것은 write path 자신의 답 — `format.*` diagnostic을 가진
  file을 다시 써야 하는 commit의 거절, 그 거절에서 면제되는 `files` file 전체 delete, 쓸 수 없는
  table — 이 그런 file을 가진 corpus에 대한 답이기 때문이다
  ([operations](40-ops.md#the-commit-protocol)).
- **저장 동등성**: 하나의 dataset을 `files`, `document`, `block`으로 저장하면 같은 읽기 요청에 같은 행을
  낸다. 이를 보이는 vector는 하나의 equivalence group을 이름 붙이며, runner는 그 group이 같은 요청과
  같은 기대 답을 가진 `files`, `document`, `block` vector를 하나씩 담을 것을 요구한다.
- **operation의 idempotency**: 읽기 vector를 반복하면 같은 답을 낸다. 거절된 쓰기를 반복하면 아무것도
  바꾸지 않았으므로 같은 방식으로 거절된다. 성공한 쓰기를 반복한 결과는 그 vector가 밝힌다: 거절(중복
  key, 새 상태에서의 전이), 같은 상태(행이 이미 가진 값을 set하는 update), 또는 요청이 본래 idempotent
  하지 않고 어떤 규칙도 거절하지 않는 곳의 또 다른 변경(엔진이 key를 정하는 insert). 이전 문구 — 거절,
  또는 upsert의 같은 상태 — 는 세 번째 경우를 빠뜨렸다. [constraints](50-constraints.md)의 어떤 규칙도
  그것을 거절하지 않으며, 어떤 SQL database도 identity key로의 두 번째 insert를 거절하지 않는다.
- **결정성**: 읽기는 저장 순서나 IR `order`를 따른다. 요청이 `random`으로 정렬하는 vector는 순서가
  아니라 행 집합과 개수를 비교한다.

## Vector 형태

vector는 `tests/conformance/vectors/` 아래의 JSON file 하나이며 `name`을 따라 이름 붙는다. 형식이
이름 붙이지 않은 member는 거절되므로, 철자가 틀린 기대는 실행되지 않는 check가 아니라 실패다.

- `name`: vector의 이름, `.json`을 뺀 file 이름.
- `storage`: 요청 entity의 저장 — `files`, `document list`, `document table`, `block list`,
  `block table` — 이며 runner가 catalog와 대조하므로 각 저장의 coverage가 밝혀지고 확인된다.
- `catalog`: `tests/conformance/catalogs/` 아래 catalog의 이름이며 corpus 전의 `dbspec/<catalog>.dbs`로
  쓰인다. vector는 catalog를 공유하므로 한 저장의 한 dataset은 한 번 적힌 하나의 법을 갖는다.
- `corpus`: corpus 전의 문서, corpus path에서 그 text로. text는 정확한 byte의 JSON 문자열이거나, 각각
  line feed로 끝나는 줄의 배열이다. 배열은 vector를 읽기 쉽게 하고, 문자열은 배열이 적을 수 없는
  byte(마지막 line feed 없음, carriage return)를 적는다.
- `diagnostics`: 선택이며, corpus path에서 corpus 전이 그 file에 가진 validator 규칙으로
  ([canonical format](10-canonical-format.md#diagnostics)). runner는 corpus 전이 정확히 이 diagnostic을
  가질 것을 요구한다. 다른 모든 file은 아무것도 갖지 않는다.
- `equivalent`: 선택이며, vector의 저장 equivalence group 이름.
- `request`: `ir_version`, `manifest_hash`, `n_params`를 뺀 IR 요청이며, runner가 IR version, catalog의
  manifest hash, `params`로 채운다.
- `params`: 요청 메시지의 parameter 값.
- `now`: 요청 메시지의 요청 clock, datetime wire 형식.
- `expect`: 답이며 두 형태 중 하나다.
  - `error`: 거절된 요청의 orm code, 그리고 선택으로 `diagnostic` — 거절이 담은 validator 규칙(쓸 수
    없는 table, rewrite 거절). corpus 후는 byte 단위로 corpus 전이다.
  - 성공한 답의 member — `rows`, `count`, `value`, `affected`, `keys` — 중 답이 가진 것 정확히 그것.
    `rows`는 projection이다: 각 기대 행은 비교할 column을 이름 붙이며, 요청이 `random`으로 정렬하지
    않는 한 답의 순서를 따른다. 나머지는 전체로 비교한다. `corpus`는 요청이 바꾼 file을 이름
    붙이며, corpus path에서 후의 text로, 또는 제거된 file이면 `null`로. 이름 붙지 않은 file은 byte를
    유지하며, 이름 붙지 않은 file은 나타나지 않는다.
- `repeat`: 성공하는 쓰기에는 필수이며, corpus 후에서 같은 요청 메시지의 결과를 `expect` 형태로 적고
  그 `corpus`는 반복이 바꾼 file을 이름 붙인다. 읽기와 거절된 쓰기는 이것을 갖지 않는다: runner는
  그 반복이 첫 run과 같이 답할 것을 요구한다.

runner는 catalog와 corpus 전을 임시 root에 쓰고, catalog를 load하고, 선언된 diagnostic을 확인하고,
corpus를 write path로 열고, session에서 요청을 실행한 뒤 답, corpus 후 — byte와 validate로 — 그리고
반복을 비교한다. journal 자신의 folder인 `.flowmark/`는 corpus file이 아니며, runner는 각 run 뒤 그
안에 stage가 없을 것을 요구한다.
