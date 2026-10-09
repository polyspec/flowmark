<!-- doc-id: spec-constraints -->
<!-- source-sha256: 7444a978e5c865b44c8acec55c925aa841517e85a7f640174357c0dce4de7187 -->
# 제약

[English](50-constraints.md)

이 명세는 validator가 corpus 위에서 강제하는 것과, DB 보증이 writer 독점을 전제로 했는데 사람의
파일 편집이 그 전제를 깨는 기능들의 판정을 확정한다.

## Parse와 type

- **Strict parse.** [grammar](10-canonical-format.ko.md) 밖의 key·line·block은 located
  diagnostic이다; lenient mode도 없고 보이지 않는 data도 없다.
- **Type.** frontmatter 값은 dbspec type 형식을 따른다; type에 실패하는 값은 그 자리에서
  diagnostic이다.

## Key와 참조

- **Primary key / unique.** canonical 문자열 형식으로 corpus 전체에서 강제한다; 중복은 두 번째
  위치에서 보고되고 첫 번째를 가리킨다.
- **Foreign key.** NFC 정규화 후 exact match로 해석한다; dangling 참조는 참조 위치에서
  diagnostic이다. 참조 action은 restrict뿐이다 — 아래 판정 참조.
- **soft-delete된 row는 자기 unique key를 계속 차지하고 foreign key를 계속 만족한다** (SQL
  동등성). soft delete가 가짜 dangling 판정을 만들지 않고 같은 key의 재insert도 여전히 중복으로
  실패한다.

## State machine

- state machine은 **dbspec-level manifest setting**이다 — terminal과 전환별 필수 field를 가진
  전환 table이며, `audit` setting이 그렇듯 자체 sub-grammar로 표현된다. SQL backend는
  `navigation`처럼 무시하고 markdown validator가 강제한다.
- table 밖의 전환은 row와 현재 상태와 거부된 대상을 밝히는 diagnostic이다. terminal 상태의
  갱신은 거부된다 — 그 대신 무엇을 할지는 application이 인코딩한다 (checklist application은
  terminal 항목을 되열지 않고 superseded 항목을 만든다).
- 전환별 필수 field는 그 전환 자체에서 검증한다; checklist application은 우회와 불가에 원인을,
  우회에 재시도 조건을 요구한다.

## 사람 write 판정

database는 writer 독점을 강제했지만 corpus는 그렇지 않다. 기능별로:

- **Foreign-key action**: restrict만 허용, `cascade`와 `set_null`은 schema parse에서 거부 —
  executor를 우회하는 writer에게 구현 불가능하다 (사람이 부모 파일을 지우면 참조가 끊긴 row가 남고,
  validator가 보고하며 해결 전까지 corpus가 거부한다).
- **`audit`와 `immutable`은 advisory다**: 위반은 validate 시점 거부이고 사람 편집에 대한
  trigger 시점 예방은 없다.
- **기본값**: non-null 기본값 column이 빠진 사람 작성 파일은 validate가 거부하고 `flowmark
  ingest`가 기본값을 명시적으로 찍는다.

## Secret과 key 없는 실행

codec column을 선언한 schema의 corpus 검증은 **key를 요구한다**: key 없는 실행은 그 column에서
근사하지 않고 실패한다. 그런 corpus의 CI는 key를 제공한다.

## Diagnostic

모든 규칙은 `{file, line, column, rule, message}`를 보고하고 rule 이름은 `schema.drift`,
`type`, `key.duplicate`, `reference.dangling`, `state.transition`, `state.required`와
canonical format의 `format.*`이다. corpus 전체 검증은 corpus-global이다 — 깨진 corpus 안의
깨끗한 파일이 corpus를 유효하게 만들지 않는다.
