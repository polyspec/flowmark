<!-- doc-id: spec-conformance -->
<!-- source-sha256: e7aedb6fbfd2256ba9bb7e2b1b364fbb24b27e5a67c2ceef7187beb12c16fc08 -->
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

## T9에서 확정할 것

vector JSON 형식, runner의 보고 형식, orm `tests/dbspec` 양식을 따르는 공유 vector 집합.
