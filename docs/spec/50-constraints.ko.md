<!-- doc-id: spec-constraints -->
<!-- source-sha256: e966f43ba82f46a0dce7fa758a5b6b51f09562070ead803ed107515cdc085499 -->
# Constraints

[English](50-constraints.md)

이 spec은 엔진이 쓰기에서 강제하는 것과 validator가 corpus 위에서 보고하는 것을 고정하며, 엔진을
우회하는 작성자 — file을 편집하는 사람 — 에 대한 판정도 포함한다. 실패 code는
[operations](40-ops.md)의 것이다.

## Parse와 type

- **Strict parse.** [문법](10-canonical-format.md)을 벗어난 줄, 블록, 섹션, key는 위치가 있는 진단이다.
  관대한 mode도, 보이지 않는 데이터도 없다.
- **Type.** 모든 값은 canonical literal 형식으로 자기 dbspec type을 따른다. type을 어긴 값은 그 자리의
  진단이고 쓰기에서 `CONSTRAINT`다.

## Key와 참조

- **primary key와 unique key**는 table 전체에서 강제한다. 중복은 쓰기에서 `DUPLICATE_KEY`이고, 두 번째
  자리에서 첫 자리를 가리키는 진단이다. unique key는 SQL처럼 NULL을 서로 다르게 본다.
- **foreign key**는 NFC 정규화 뒤 정확히 일치로 풀린다. 어떤 행도 가리키지 않는 참조는 쓰기에서
  `FOREIGN_KEY`이고 참조 자리의 진단이다.
- **참조 action은 restrict-only다.** `cascade`와 `set_null`은 catalog를 읽을 때 거절한다: 엔진을
  우회하는 작성자에게는 구현할 수 없다. 참조되는 행 — 이력 행을 가진 행 포함 — 의 삭제는
  `FOREIGN_KEY`다.
- **soft delete된 행도 unique key를 차지하고 foreign key를 만족한다**(SQL과 같음). 그래서 soft delete가
  가짜 dangling 판정을 만들지 않고, 같은 key의 재insert는 여전히 중복으로 실패한다.

## Check

check는 commit에서 강제한다. 술어가 unknown(NULL)인 check는 SQL처럼 통과한다.

## 계층과 순서

- `parent` 순환은 `CONSTRAINT`다. 행의 블록 밖 `parent`는 쓰기에서 `CONSTRAINT`, 읽기에서
  `format.block`이다.
- 1..n + 1 밖의 순서 값이나, 둘 이상의 행과 맞는 order column update는 `CONSTRAINT`다. `files` 저장에서
  정확히 1..n이 아닌 형제 순서 값은 진단이다([document model](20-document-model.md)).

## 상태기계

- **Insert.** insert에서 행의 상태는 초기 상태다. 기계의 상태 집합 밖의 상태는 `CONSTRAINT`다.
- **전이.** 상태 column의 update는 선언된 전이를 따른다. 다른 변경은 행, 현재 상태, 거절된 목표를
  밝히는 `CONSTRAINT`다. 종료 상태는 떠나지 않는다 — 대신 할 일은 응용이 정한다(응용 schema는 종료된
  항목을 다시 여는 대신 `supersedes` link와 함께 새 항목을 insert한다).
- **필수 column.** 줄에 `require (c1, …)`를 가진 전이나 종료 상태 진입은 같은 update에서 나열된 모든
  column을 할당한다. 할당되지 않은 column, `null`, 빈 텍스트는 그 column을 밝히는 `CONSTRAINT`다.
  상태 table은 그 값을 유지한다.
- **이력.** 엔진이 실행하는 모든 전이는 같은 transaction에서 이력 행 하나를 insert한다: 행의 foreign
  key, from과 to 상태, `at`, 필수 column의 사본. insert는 이력을 쓰지 않는다.
- **Limit.** `limit <state> <n>`은 그 상태의 table 행을 센다. 이를 넘게 할 전이, insert, update는
  limit를 밝히는 `CONSTRAINT`다. 사람의 편집 뒤 이미 넘은 corpus는 다른 쓰기를 막지 않는 진단이다.

## 엔진 밖에서 바뀐 상태

- **빈틈.** 행의 상태가 마지막 이력 행의 `to`와 다르거나, 이력 행이 없을 때 상태가 초기 상태가 아니면
  이력 빈틈이다. validate는 행의 줄에서 `history.gap`을 보고한다. 빈틈 있는 행의 엔진 전이는 빈틈이
  닫힐 때까지 `CONSTRAINT`이고, 그 행의 다른 column update는 허용한다.
- **사슬.** 이력 행의 `from`이 이전 행의 `to`와 다르거나 — 첫 행이면 초기 상태가 아니거나 — 그 쌍이
  선언된 전이가 아니면 validate는 `history.chain`을 보고한다.
- 두 진단 모두 다른 쓰기를 막지 않는다. 그러므로 엔진 밖의 상태 변경은 사람이 일관된 이력을 위조하지
  않는 한 보고된다.
- **Repair.** `flowmark history repair <table> <key> [--from <initial state>]`는 마지막 `to`에서 — 이력
  없는 행은 하나뿐인 초기 상태, 또는 초기 상태가 여럿이면 필수인 `--from`에서 — 현재 상태로 가는 이력
  행 하나를 insert한다. `at`은 엔진 시계이고 필수 column은 행에서 복사한다. 그 쌍이 선언된 전이가
  아니거나, 전이가 요구하는 column이 행에서 null이거나 비었거나, 이 행을 세어 현재 상태의 limit를 넘으면
  거절하며, 그때 사람이 편집으로 합법 상태를 되돌리거나 column을 채운다. repair는 사람이 commit하는 보통
  transaction이다.

## 사람이 쓴 편집에 대한 판정

database는 작성자 독점을 강제하지만 corpus에는 그것이 없다. 기능별로:

- **foreign key action**: 위와 같이 restrict-only다. 사람이 부모 file을 지우면 dangling 참조가 남고
  validate가 보고한다.
- **`audit`와 `immutable`**은 corpus에서 schema 오류다 — markdown에는 trigger가 없다. 상태기계의 이력이
  기록된 흔적이고, 그 빈틈과 사슬 규칙이 사람의 편집이 우회한 것을 보고한다.
- **Default**는 엔진이 insert에서 명시적으로 쓴다. non-null column이 빠진 사람이 쓴 행은 format 진단이다.

## 진단

모든 규칙은 `{file, line, column, rule, message}`를 보고한다. 규칙 이름은 `catalog`, `type`,
`key.duplicate`, `reference.dangling`, `check`, `parent.cycle`, `order`, `state.transition`,
`state.required`, `state.limit`, `history.gap`, `history.chain`, 그리고 canonical format의 `format.*`
규칙이다. 검증은 corpus 전체에 걸친다. 깨진 corpus 안의 깨끗한 file이 corpus를 유효하게 만들지 않는다.
