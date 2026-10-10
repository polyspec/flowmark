<!-- doc-id: spec-constraints -->
<!-- source-sha256: 1b9f4d91d31bce195f0532efaab880dcade9bddb4f62e736cc952daec56437cb -->
# Constraints

[English](50-constraints.md)

이 spec은 엔진이 쓰기에서 강제하는 것과 validator가 corpus 위에서 보고하는 것을 고정하며, 엔진을
우회하는 작성자 — file을 편집하는 사람 — 에 대한 판정도 포함한다. 실패 code는
[operations](40-ops.md)의 것이다.

## Parse와 type

- **Strict parse.** [문법](10-canonical-format.md)을 벗어난 줄, 블록, 섹션, key는 위치가 있는 진단이다.
  관대한 mode도, 보이지 않는 데이터도 없다.
- **Type.** 모든 값은 [canonical format](10-canonical-format.md)의 읽을 수 있는 형식으로 자기 dbspec type을 따른다. type을 어긴 값은 그 자리의
  진단이 validator의 `type`이고, 값을 배정하는 쓰기에서 `CONSTRAINT`다([canonical format](10-canonical-format.md)).

## Key와 참조

- **primary key와 unique key**는 table 전체에서 강제한다. 중복은 그것을 새로 만드는 쓰기에서 `DUPLICATE_KEY`이고, 두 번째 자리에서 첫 자리를 가리키는 validator의 `key.duplicate` 진단이다. unique key는 SQL처럼 NULL을 서로 다르게 본다.
- **foreign key**는 참조된 key와 byte 단위로 정확히 일치해야 풀린다. 어떤 행도 가리키지 않는 참조는
  dangling 참조다: 그것을 새로 만드는 쓰기는 `FOREIGN_KEY`이고, validator의 `reference.dangling`이 참조 자리에 보고하며, file을 읽을 수 없게 하지 않는다. 다만 block이나 document 행의 `parent`가 그 file의 어떤 행도 가리키지 않으면 `format.block`이다([canonical format](10-canonical-format.md#foreign-keys-as-links)).
- **참조된 행의 key 갱신**은 `FOREIGN_KEY`(restrict-only)다. 그래서 link의 target은 link 아래에서
  움직이지 않는다.
- **참조 action은 restrict-only다.** `cascade`와 `set_null`은 catalog를 읽을 때 거절한다: 엔진을
  우회하는 작성자에게는 구현할 수 없다. 참조되는 행 — 이력 행을 가진 행 포함 — 의 삭제는
  `FOREIGN_KEY`다.
- **soft delete된 행도 unique key를 차지하고 foreign key를 만족한다**(SQL과 같음). 그래서 soft delete가
  가짜 dangling 판정을 만들지 않고, 같은 key의 재insert는 여전히 중복으로 실패한다.

## Key 규칙

- **형태.** markdown table의 key는 `A`–`Z`, `a`–`z`, `0`–`9`, `_`, `-`로 이루어지고 문자나 숫자로
  시작하며, `--`와 `-_`를 담지 않는다. 이 형태 밖의 key는 쓰기에서 `CONSTRAINT`다.
- **예약 이름.** 대소문자를 무시하고 `CON`, `PRN`, `AUX`, `NUL`, `COM0`–`COM9`, `LPT0`–`LPT9`와 같은
  key는 쓰기에서 `CONSTRAINT`다([schema](30-schema.md)는 table 이름을 거절한다).
- **길이.** 243 byte를 넘는 `files` key는 쓰기에서 `CONSTRAINT`다.
- **대소문자 무시 유일성.** 대소문자를 무시하고 table의 다른 key와 같은 key는 그것을 새로 만드는 쓰기에서 `DUPLICATE_KEY`이며, 첫 key를 밝힌다. validator의 `key.duplicate`이기도 하다. 충돌은 key의 file 이름과 anchor가 중복된 것이며, file을 읽을 수 없게 하지 않는다. 다만 같은 file 안에서 바이트 단위로 같은 key를 가진 두 행은, 둘 다 상세 섹션을 가지면 두 섹션이 같은 heading 텍스트를 가지므로([canonical format](10-canonical-format.md#detail-headings-and-anchors)), 또는 그 file의 `parent` cell이 그 key를 가리키면 부모를 배정할 수 없으므로([canonical format](10-canonical-format.md#table-rows)) `format.block`이다; 바이트 단위로 같은 key를 가진 두 행은 그 table을 사용할 수 없게 한다([canonical format](10-canonical-format.md#diagnostics)).
- **정식 file 이름.** key 타입의 정식 텍스트가 아닌 `files` file 이름은 읽기에서 `format.block`이다
  ([document model](20-document-model.md)).

## 읽을 수 있는 문서의 거절

쓰기에서 다음 각각은 `CONSTRAINT`다. 괄호 안은 그것을 정하는 규칙이다:

- 줄바꿈이나 CR, 앞이나 뒤의 공백, 또는 `<` 뒤에 `!`, `/`, 글자가 오는 한 줄 값 — list 제목, H1, cell, list
  field, field list 값 안에서([canonical format](10-canonical-format.md));
- 첫 줄이 `-` list 항목이거나, field list 뒤에 두 column 이상(공백 또는 tab) 들여쓴 줄인 `files` body([canonical format](10-canonical-format.md));
- `files` row의 H1 제목 텍스트가 `\#`로 끝나는 제목([canonical format](10-canonical-format.md));
- 바이트 그대로 쓸 수 없는 text나 `markdown` 값, 그리고 `CanonicalLiteral`이 거절하는 typed 값(scale보다 많은 소수 자릿수 — 0 포함, precision − scale보다 많은 정수 자릿수, precision보다 많은 소수 자릿수의 time이나 datetime)([canonical format](10-canonical-format.md));
- `format.*` 진단이 있는 file을 다시 쓰는 logical write는 file과 진단을 밝히며 `CONSTRAINT`다. 그 진단이 `format.noncanonical`이면 `flowmark canon`을 밝힌다(commit의 면제를 제외하고)([canonical format](10-canonical-format.md)). 쓰기는 문서 바이트를 정규화하지 않는다;
- CommonMark block 구조에서 최상위 level ≤ L heading을 가진 `markdown` 값([canonical format](10-canonical-format.md));
- 상세 anchor와 heading의 충돌을 새로 만드는 쓰기 — 제목 갱신, payload 쓰기, 행 삽입, key 갱신, 이동,
  상세 섹션의 생성 — 는 다른 heading을 밝히며 `CONSTRAINT`다. 다만 중복 key는 `DUPLICATE_KEY`다([canonical format](10-canonical-format.md#detail-headings-and-anchors));
- date, time 또는 datetime 값으로 쓴 `now`(그 type이 아니다), 그리고 column 길이를 넘는 text([canonical format](10-canonical-format.md));
- list 제목 token `<declared column>\:`([canonical format](10-canonical-format.md#list-rows));
- 최상위 setext heading을 담거나, 뒤따르는 요소가 그 안으로 흡수되는 fence나 HTML block을 열린 채로 둔 `markdown` 값([canonical format](10-canonical-format.md#sections-and-their-values)).

읽기에서는 walk가 `corpus.walk`를, 형식 규칙이 `format.*`를 보고한다. 그 내용은
[document model](20-document-model.md)과 [canonical format](10-canonical-format.md)이 정한다.

## Check

check는 commit에서 강제한다. 술어가 unknown(NULL)인 check는 SQL처럼 통과한다.

## 계층과 순서

- `parent` 순환은 쓰기에서 `CONSTRAINT`이고, 한 file의 행 사이의 순환은 읽기에서 `parent.cycle`이며 그 file을 읽을 수 없게 한다([document model](20-document-model.ko.md#계층)); 여러 `files` file의 행에 걸친 순환은 validator의 `parent.cycle`뿐이다. 행의 블록 밖 `parent`는 쓰기에서 `CONSTRAINT`, 읽기에서
  `format.block`이다.
- 1..n + 1 밖의 순서 값(n은 문서 모델이 정의하는 대로)이나, 둘 이상의 행과 맞는 order column update는
  `CONSTRAINT`다. block과 document 저장에서 쓰기 뒤의 형제 묶음 순서 값이 정확히 1..m(m은 그 묶음의 쓰기 뒤 크기)이 아니면 `CONSTRAINT`다. `files` 저장에서 정확히 1..n이 아닌 형제 순서 값은 진단이다([document model](20-document-model.md)).

## 상태기계

- **Insert.** insert에서 행의 상태는 초기 상태다. 기계의 상태 집합 밖의 상태는 `CONSTRAINT`다.
- **전이.** 상태 column의 update는 선언된 전이를 따른다. 다른 변경은 행, 현재 상태, 거절된 목표를
  밝히는 `CONSTRAINT`다. 종료 상태는 떠나지 않는다(dbspec의 terminal 의미) — 대신 할 일은 응용이 정한다(응용 schema는 종료된
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
- **Commit.** commit은 먼저 그 대상이 읽은 뒤 바뀌지 않았는지 검사한다: 바뀌었으면 `WRITE_CONFLICT`다.
  이력 빈틈(`history.gap`)을 포함한 constraint 검사는 잠금 아래에서 다시 읽은 corpus 위에서 실행한다.
  다른 행의 validator 진단(`type`, `key.duplicate`, `reference.dangling`, `check`, `order`, `state.limit`, `history.gap`, `history.chain`, 여러 `files` file에 걸친 `parent.cycle`)은 commit을 실패시키지 않되, `type` 진단은 commit의 제약 검사가 그 값을 평가해야 할 때 commit을 실패시키고, 바이트 단위로 같은 key를 가진 두 행은 그 table을 사용할 수 없게 한다([canonical format](10-canonical-format.md#diagnostics)); `format.*` 진단은 다시 쓰기 거절과 사용할 수 없는 table 상태([canonical format](10-canonical-format.ko.md#진단))가 정하는 경우에만 commit을 실패시킨다. file을 다시 써야 하는 commit — 행 자신의 file, 상세 섹션이 생기거나 사라지는 행으로의 link를 가진 file, `order`의 형제 — 은 그 file이 `format.*` 진단을 가지면 거절한다: `CONSTRAINT`이며 file과 진단을 밝히고, `format.noncanonical`이면 `flowmark canon`을 밝힌다. 세 commit은 면제된다: 대상에 대한 `flowmark canon`([operations](40-ops.ko.md#canon-연산)), 추가된 column이 일으킨 진단에 대한 add-column migration([migration](80-migration.ko.md)) — 그 진단은 그 migration에 대해 table을 사용할 수 없게 하지 않는다 —, `files` 행의 file 전체를 지우는 delete는 rewrite 거절에서만 면제되며, restrict 검사는 그 행을 참조할 수 있는 모든 table의 행을 여전히 필요로 하며(자신의 table을 `parent`로 하는 것, 자식 및 이력 block, 그 행으로의 foreign key를 가진 모든 table), 그 중 하나라도 사용할 수 없으면 그 delete는 `CONSTRAINT`다.
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

모든 규칙은 `{file, line, column, rule, message}`를 보고하며, column은 [canonical format](10-canonical-format.ko.md#진단)이 정하는 대로다. 규칙 이름은 `catalog`, `type`,
`key.duplicate`, `reference.dangling`, `check`, `parent.cycle`, `order`, `state.transition`,
`state.required`, `state.limit`, `history.gap`, `history.chain`, `corpus.walk`
([document model](20-document-model.md)의 walk 규칙), 그리고 canonical format의 `format.*` 규칙이다: `format.noncanonical`, `format.block`,
`format.section`, `format.encoding`. 검증은 corpus 전체에 걸친다. 깨진 corpus 안의 깨끗한 file이 corpus를
유효하게 만들지 않는다. 열기, introspection, `flowmark validate`, `flowmark canon`은 행에 대한 요청이 아니므로 사용할 수 없는 table에서 결코 실패하지 않으며, canon의 링크 유도 거절은 예외다([operations](40-ops.ko.md#canon-연산)). validator는 모든 file의 `format.*`와 `parent.cycle` 진단과, `format.block`, `format.section`, `format.encoding`이나 그 행 사이의 `parent.cycle`이 없는 file 안에서 읽을 수 있는 모든 행의 validator 규칙을 보고한다. `format.block`, `format.section`, `format.encoding`이나 그 행 사이의 `parent.cycle`이 있는 file을 가진 table의 완전한 행을 필요로 하는 규칙 — 그 table로의 `reference.dangling`, `key.duplicate`, 그 안의 `order`와 `state.limit` — 은 그 table에 대해 보고하지 않으며, file 자신의 진단이 그것을 대신한다. 바이트 단위로 같은 key 때문에만 사용할 수 없는 table은 모든 규칙이 보고된다.
