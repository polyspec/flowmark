<!-- doc-id: spec-conformance -->
<!-- source-sha256: 0cc4dabd8c52c7472509ce02add05960165a14ec1a02cbb6ac4328821f0839ab -->
# Conformance (범위를 정한 뼈대)

[English](90-conformance.md)

이 문서는 엔진의 conformance 계약을 적는다. vector 형식의 세부는 다시 쓴 vector와 함께 정한다.

## 계약

- vector는 **corpus 전 — 그 catalog와 문서 — 과 요청, 그리고 기대하는 corpus 후, 행, 또는 오류**다.
  비교할 SQL 텍스트나 bind 목록은 없다. SQL conformance harness는 적용되지 않으며 이 계약이 그것을
  대신한다.
- 요청은 [capability table](60-dialect.md) 아래의 IR 요청이거나, vector가 face를 시험하는 경우 face
  operation이다.
- vector는 flowmark 엔진을 상대로 실행한다. orm의 `markdown` IR executor conformance는 orm에 고정한
  flowmark build를 상대로 실행되며 orm 자신의 계약이다.
- 모든 vector는 이름을 밝히며, 실행할 수 없는 run은 orm conformance 전통에 따라 실패다.

## 요구 성질

- **Round trip**: 모든 vector의 corpus 전은 모든 저장에서 canonical이다(`emit(parse(s)) == s`). corpus
  후는 구성상 canonical이다.
- **저장 동등성**: 하나의 dataset을 `files`, `document`, `block`으로 저장하면 같은 읽기 요청에 같은 행을
  낸다.
- **operation의 idempotency**: 읽기 vector를 반복하면 같은 행을 낸다. 쓰기 vector를 반복하면 거절되거나
  (중복 key, 새 상태에서의 전이) operation이 upsert인 곳에서는 같은 상태를 내며, vector는 어느 쪽인지
  밝힌다.
- **결정성**: 읽기는 저장 순서나 IR `order`를 따른다. `orderByRandom` vector는 순서가 아니라 행 집합과
  개수를 비교한다.

## Vector 형태

vector는 `tests/conformance/vectors/` 아래의 JSON file 하나다: `name`, `corpus`(`dbspec/`과 그 문서를
가진 directory), `request`, `expect` — key projection인 `rows`, 기대하는 file인 `corpus`, 또는 orm code인
`error`. runner는 corpus를 임시 root에 복사해 열고 요청을 실행한 뒤 기대와 비교한다. 기대하는 행은
column을 이름 붙이므로 행 비교는 전체 행 동등이 아니라 projection이다.
