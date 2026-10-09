<!-- doc-id: spec-conformance -->
<!-- source-sha256: e07eb2914dbc3332a62e0fd3c9a71e106d07b228589c6132a140cd48291fbdcb -->
# Conformance (범위 skeleton)

[English](90-conformance.md)

이 문서는 conformance 계약을 서술한다. vector 형식의 세부 사항은 T9에서 첫 vector와 함께
작성한다.

## 계약

- markdown vector는 **corpus before + IR request + 기대 corpus after / 기대 rows**다. 비교할 SQL
  text와 bind list가 없다 — SQL conformance harness는 이 backend에 적용되지 않고 이 계약이 그것을
  대체한다.
- vector는 Go executor에서 실행한다. 다른 client가 executor를 port할 때 교차 비교가 시작된다.
  그 전까지 capability table은 markdown 지원을 Go-only로 표시한다.
- 모든 vector는 이름을 가진다. 실행할 수 없는 run은 실패다 — orm conformance 관례대로.

## 필수 속성

- **round-trip**: 모든 vector의 corpus-before는 canonical이다 (`emit(parse(s)) == s`).
  corpus-after는 구성상 canonical이다.
- **연산의 멱등성**: read vector의 반복은 같은 rows를 반환한다. write vector의 반복은 conflict로
  거부된다 (또는 upsert 연산인 경우 같은 상태를 생성한다). vector가 어느 쪽인지 명시한다.
- **결정성**: ordering은 type-directed 규칙을 따른다. `orderByRandom` vector는 revision seed를
  고정한다.

## vector 형식

vector는 `tests/conformance/vectors/` 아래의 JSON 파일 하나다: `name`, `corpus` (경로→문서
text), `call` (kind, table, query), `expect` (key projection인 rows, count, 또는 page와 total).
runner가 임시 directory에 corpus를 만들고 열고 call을 실행해 기대값과 비교한다. 기대 rows가
column을 지정하므로 row 비교는 projection이지 전체 row 동등성이 아니다.
