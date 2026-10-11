<!-- doc-id: spec-constraints -->
<!-- source-sha256: 1a0c80942a8c645ca2ee7d158d16394421923d65f7d473354c128a9cb76beaa3 -->
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

- **primary key와 unique key**는 table 전체에서 강제한다 — block table이면 그것을 가진 모든 file의 모든 block에 걸쳐서. 중복은 그것을 새로 만드는 쓰기에서 `DUPLICATE_KEY`이고, validator의 `key.duplicate`는 [corpus 순서](10-canonical-format.ko.md#진단)에서 첫 자리 다음의 각 자리에, 그 key에 있으며 첫 자리를 밝힌다. unique key는 SQL처럼 NULL을 서로 다르게 본다. unique key의 validator `key.duplicate`는 값을 각각 정식 readable 형식으로 바이트 단위로 비교하며, corpus 순서에서 첫 자리 다음의 각 자리에, key의 첫 column에 있고 첫 자리를 밝힌다. 그 column 중 어느 것의 값이 NULL이거나 column의 type이 아닌 행은 비교에 참여하지 않는다. 근거: SQL과 같다 — orm은 모든 문자 column에 code point를 비교하는 binary collation 하나(`utf8mb4_0900_bin`, `COLLATE "C"`, `BINARY`)를 주고 NULL의 반복을 허용한다 — 그리고 엔진이 읽지 못하는 값에는 비교가 없으며 그 `type`이 그것을 대신한다. primary key가 대소문자를 무시하고 비교하는 것은 그것이 file 이름이자 anchor이기 때문일 뿐이다([document model](20-document-model.ko.md#key)). unique key의 값은 어느 쪽도 아니므로 그 이유가 미치지 않는다.
- **foreign key**는 참조된 key와 byte 단위로 정확히 일치해야 풀린다. 어떤 행도 가리키지 않는 참조는
  dangling 참조다: 그것을 새로 만드는 쓰기는 `FOREIGN_KEY`이고, validator의 `reference.dangling`이 참조 자리에 보고하며, file을 읽을 수 없게 하지 않는다. 다만 block이나 document 행의 `parent`가 그 file의 어떤 행도 가리키지 않으면 `format.block`이다([canonical format](10-canonical-format.md#foreign-keys-as-links)).
- **참조된 행의 key 갱신**은 `FOREIGN_KEY`(restrict-only)다. 그래서 link의 target은 link 아래에서
  움직이지 않는다.
- **참조 action은 restrict-only다.** `cascade`와 `set_null`은 catalog를 읽을 때 거절한다: 엔진을
  우회하는 작성자에게는 구현할 수 없다. 참조되는 행 — 이력 행을 가진 행 포함 — 의 삭제는
  `FOREIGN_KEY`다. restrict는 쓰기 전의 행에 대해 성립한다: delete가 지우는 행이 참조되면, 같은 요청이
  지우는 행이 참조하더라도 `FOREIGN_KEY`이며, 참조하는 행은 앞선 요청이 지운다. 근거: orm은 즉시
  검사하는 `RESTRICT`를 render한다. 그러면 MySQL과 SQLite는 그런 delete를 자기 행 순서에 따라 거절하거나
  받아들이고 PostgreSQL은 받아들이는데, 집합 delete에는 행 순서가 없으므로 쓰기 전의 검사가 순서와
  무관한 유일한 규칙이다.
- **soft delete된 행도 unique key를 차지하고 foreign key를 만족한다**(SQL과 같음). 그래서 soft delete가
  가짜 dangling 판정을 만들지 않고, 같은 key의 재insert는 여전히 중복으로 실패한다.

## Key 규칙

- **형태.** markdown table의 key는 `A`–`Z`, `a`–`z`, `0`–`9`, `_`, `-`로 이루어지고 문자나 숫자로
  시작하며, `-_`를 담지 않는다. 이 형태 밖의 key는 쓰기에서 `CONSTRAINT`다.
- **예약 이름.** 대소문자를 무시하고 `CON`, `PRN`, `AUX`, `NUL`, `COM0`–`COM9`, `LPT0`–`LPT9`와 같은
  key는 쓰기에서 `CONSTRAINT`다([schema](30-schema.md)는 table 이름을 거절한다).
- **길이.** 243 byte를 넘는 `files` key는 쓰기에서 `CONSTRAINT`다.
- **대소문자 무시 유일성.** 대소문자를 무시하고 table의 다른 key와 같은 key는 그것을 새로 만드는 쓰기에서 `DUPLICATE_KEY`이며, 다른 행을 밝힌다. validator의 `key.duplicate`이기도 하다. 충돌은 key의 file 이름과 anchor가 중복된 것이며, file을 읽을 수 없게 하지 않는다. 다만 같은 file 안에서 바이트 단위로 같은 key를 가진 두 행은, 둘 다 상세 섹션을 가지면 두 섹션이 같은 heading 텍스트를 가지므로([canonical format](10-canonical-format.md#detail-headings-and-anchors)), 또는 그 file의 `parent` cell이 그 key를 가리키면 부모를 배정할 수 없으므로([canonical format](10-canonical-format.md#table-rows)) `format.block`이다; 바이트 단위로 같은 key를 가진 두 행은 그 table을 사용할 수 없게 한다([canonical format](10-canonical-format.md#diagnostics)). key가 앞선 행의 key와 바이트 단위로 같은 행에서 validator의 결함은 그런 첫 행을 밝히고 바이트 단위로 같다고 말한다. 그렇지 않으면 key가 대소문자를 무시하고 같은 첫 행을 밝힌다. 쓰기 결함처럼 한 행의 `key.duplicate`는 하나다([canonical format](10-canonical-format.ko.md#진단)).
- **정식 file 이름.** key 타입의 정식 텍스트가 아닌 `files` file 이름은 읽기에서 `format.block`이다
  ([document model](20-document-model.md)).

## 읽을 수 있는 문서의 거절

쓰기에서 다음 각각은 `CONSTRAINT`다. 괄호 안은 그것을 정하는 규칙이다:

- 줄바꿈이나 CR, 앞이나 뒤의 보이지 않는 문자(spec 10이 정한 것: U+1680을 뺀 공백, Cc, Prepended_Concatenation_Mark를 뺀 Cf, Default_Ignorable_Code_Point), 또는 `<` 뒤에 `!`, `/`, 글자가 오는 한 줄 값 — list 제목, H1, cell, list
  field, field list 값 안에서([canonical format](10-canonical-format.md));
- 첫 줄이 `-` list 항목이거나, field list 뒤에 두 column 이상(space 또는 tab) 들여쓴 줄인 `files` body([canonical format](10-canonical-format.md));
- `files` row의 H1 제목 텍스트가 `\#`로 끝나는 제목([canonical format](10-canonical-format.md));
- 바이트 그대로 쓸 수 없는 text나 `markdown` 값, 그리고 `CanonicalLiteral`이 거절하는 typed 값(scale보다 많은 소수 자릿수 — 0 포함, precision − scale보다 많은 정수 자릿수, precision보다 많은 소수 자릿수의 time이나 datetime)([canonical format](10-canonical-format.md));
- `format.*` 진단이 있는 file을 다시 쓰는 logical write는 file과 진단을 밝히며 `CONSTRAINT`다. 그 진단이 `format.noncanonical`이면 `flowmark canon`을 밝힌다(commit의 면제를 제외하고)([canonical format](10-canonical-format.md)). 쓰기는 문서 바이트를 정규화하지 않는다;
- CommonMark block 구조에서 최상위 level ≤ L heading을 가진 `markdown` 값([canonical format](10-canonical-format.md));
- 상세 anchor와의 충돌(heading 또는 HTML `id`나 `name`)을 새로 만드는 쓰기 — 제목 갱신, payload 쓰기, 행 삽입, key 갱신, 이동,
  상세 섹션의 생성 — 는 anchor를 차지하는 heading이나 id를 쓴 행과 column — `markdown` column의 값, `files` 행의 제목(H1에 대해), 또는 section이나 block heading에 대해서는 그것이 여는 행 scope의 행, 그 heading이 이름 붙인 `markdown` column이나 자식 table — 에 위치하며, 차지한 anchor의 상세 heading을 밝히는 `CONSTRAINT`다. 다만 중복 key는 `DUPLICATE_KEY`다([canonical format](10-canonical-format.md#detail-headings-and-anchors));
- date, time 또는 datetime 값으로 쓴 `now`(그 type이 아니다), 그리고 column 길이를 넘는 text([canonical format](10-canonical-format.md));
- list 제목 token `<declared column>\:`([canonical format](10-canonical-format.md#list-rows));
- 최상위 setext heading을 담거나, 뒤따르는 요소가 그 안으로 흡수되는 fence나 HTML block을 열린 채로 둔 `markdown` 값. 그 요소를 어느 쓰기가 놓든 마찬가지다([canonical format](10-canonical-format.md#sections-and-their-values)).

읽기에서는 walk가 `corpus.walk`를, 형식 규칙이 `format.*`를 보고한다. 그 내용은
[document model](20-document-model.md)과 [canonical format](10-canonical-format.md)이 정한다.

## Check

check는 commit에서 강제한다. 술어가 unknown(NULL)인 check는 SQL처럼 통과한다. validator는 table의 각 check를 읽을 수 있는 각 행에 대해 그 행의 논리 값 — 읽기가 돌려주는 대로의 모든 column, 그리고 부모 행의 key인 배치 foreign key — 으로 평가하고, 술어가 false이면 행의 첫 줄에서 check와 행을 밝혀 `check`를 보고한다. text와 `uuid`는 code point로(orm의 binary collation 하나), 수는 값으로, date, time, datetime은 시간순으로 비교한다. column의 type이 아닌 값은 `is [not] null`에서 NULL이 아니며, 그 값을 읽고 NULL operand가 없는 비교나 `in`은 한 번은 true로, 한 번은 false로 두 번 평가한다: dbspec 술어에는 `not`이 없으므로 이 atom들에 대해 단조이고, 첫 평가는 어떤 값이 줄 수 있는 가장 큰 결과, 둘째는 가장 작은 결과다. validator는 첫 평가가 false이면 — 그 값이 무엇이든 check가 false이면 — `check`를 보고하고, 나머지는 그 값의 `type`이 대신한다. commit은 첫 평가가 false이면 check를 밝히는 `CONSTRAINT`, 둘째만 false이면 check가 그 값을 필요로 하므로 `type` 진단을 밝히는 `CONSTRAINT`이고, 그 밖에는 통과한다. 근거: check는 자기 행의 술어이므로 행을 읽을 수 있는 곳이면 사용할 수 없는 table에서도 보고한다. SQL은 어긴 check를 column이 아니라 그 이름으로 밝힌다.

## 계층과 순서

- `parent` 순환은 쓰기에서 `CONSTRAINT`이고, 한 file의 행 사이의 순환은 읽기에서 `parent.cycle`이며 그 file을 읽을 수 없게 한다([document model](20-document-model.ko.md#계층)); 여러 `files` file의 행에 걸친 순환은 validator의 `parent.cycle`뿐이다. 행의 블록 밖 `parent`는 쓰기에서 `CONSTRAINT`, 읽기에서
  `format.block`이다.
- 1..n + 1 밖의 순서 값(n은 문서 모델이 정의하는 대로)이나, 둘 이상의 행과 맞는 order column update는
  `CONSTRAINT`다. block과 document 저장에서 쓰기 뒤의 형제 묶음 순서 값이 정확히 1..m(m은 그 묶음의 쓰기 뒤 크기)이 아니면 `CONSTRAINT`다. `files` 저장에서 validator의 `order`는 순서 값이 1..n 밖이거나 형제와 공유되는 각 행에, 그 `order` 항목에 있으며(n은 그 형제 묶음의 크기), column의 type이 아닌 값은 `type`뿐이다: 그 값은 n에 세어지며 1..n 밖도 공유도 아니다([document model](20-document-model.ko.md#순서)). 근거: 읽을 수 없는 값에는 비교가 없으므로 type 결함이 그것을 대신하며, 그 값이 무엇이든 다른 형제가 가지는 결함은 여전히 보고된다.
- `parent`가 그 column의 type이 아닌 행은 알려진 형제 묶음에 속하지 않는다: 어느 묶음의 n에도 세어지지 않고 자신의 `order`는 판정하지 않으며, 묶음의 결함은 그런 행들이 어느 묶음에 들어가든 성립할 때만 보고한다 — 묶음 안에서 공유되는 값, 또는 1..n + u 밖의 값(u는 그런 행의 수). 나머지는 그 행의 `type`이 대신한다.

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
  limit를 밝히는 `CONSTRAINT`다. 이 수는 soft delete된 행을 포함한다: soft delete된 행은 여전히 unique
  key를 차지하듯 그 상태에 있는 저장된 행이므로, soft delete는 자리를 비우지 않고 restore는 limit를
  넘지 않는다. 사람의 편집 뒤 이미 넘은 corpus는 다른 쓰기를 막지 않는 validator의 `state.limit`다: table을 가진 모든 block과 file에서 읽을 수 있는 table 행 m개가 그 상태에 있고 m > n이면, [corpus 순서](10-canonical-format.ko.md#진단)에서 n번째 다음의 각 행을 그 상태 자리(list 행의 glyph, 상태 cell, `files` field list의 상태 항목)에 보고하며, 상태와 limit와 m을 밝힌다. 근거: limit는 그 상태에 n개 행을 허용하므로 m − n개 행이 그 너머에 있고 각각이 결함 하나다. `key.duplicate`가 첫 자리 다음의 각 자리에 보고되는 것과 같다([Key](#key와-참조)). 그것이 어느 행인지는 내용으로 정해지지 않는다 — 사람의 편집은 어느 행이 마지막으로 그 상태에 들어왔는지 기록을 남기지 않으며, 그것은 `history.gap`이 보고하는 것이다 — 그래서 corpus만의 함수인 순서로 고른다.

## 엔진 밖에서 바뀐 상태

- **빈틈.** 행의 상태가 마지막 이력 행의 `to`와 다르거나, 이력 행이 없을 때 상태가 초기 상태가 아니면
  이력 빈틈이다. validate는 행의 줄에서 `history.gap`을 보고한다. 빈틈 있는 행의 엔진 전이는 빈틈이
  닫힐 때까지 `CONSTRAINT`이고, 그 행의 다른 column update는 허용한다.
- **사슬.** validate는 `from`이 이전 행의 `to`와 다른 — 첫 행이면 초기 상태가 아닌 — 이력 행에
  `history.chain`을 보고한다. 근거: 이력 행은 행 상태의 사슬이므로, 그 끊김은 어떤 이력 행도 기록하지
  않은 상태 변경이다.
- **전이.** validate는 쌍이 선언된 전이가 아닌 이력 행에 `state.transition`을 보고한다. 한 행이
  `history.chain`과 `state.transition`을 함께 가질 수 있으며, 결함마다 보고 하나다. 근거: 엔진은 선언된
  전이만 실행하므로 선언되지 않은 쌍은 엔진이 밟지 않은 단계이고, 행이 앞 행을 잇는지와 그 쌍이
  선언되었는지는 행에 대한 별개의 두 사실이다.
- **필수.** validate는 쌍이 선언되었고, 그 전이의 `require` 목록이나 — `to`가 종료 상태이면 — 그 종료
  상태의 `require` 목록이 밝히는 column에 NULL이나 빈 텍스트를 가진 이력 행에 `state.required`를
  보고한다 — 그 column의 요소에, 행에 그 요소가 없으면(없는 섹션, 빈 cell) 행의 첫 줄에. 쌍이 선언되지
  않은 행은 `state.transition`만이다. 근거: 엔진이 실행하는 모든 전이는 필수 column을 그 이력 행에
  복사하고 거기서 NULL과 빈 텍스트를 거절하므로([필수 column](#상태기계)) 그런 행은 엔진이 쓴 것이
  아니다. 어떤 column이 필수인지는 선언된 쌍에서만 알 수 있다.
- **type만.** column의 type이 아닌 상태, `from`, `to`는 `type`만이다: 그 값 위에서는 `history.gap`,
  `history.chain`, `state.transition`, `state.required`를 판정하지 않는다 — 그 값이 상태나 마지막 `to`인
  행의 빈틈도, 그 다음 이력 행의 사슬도. 근거: `order`와 foreign key처럼 결함 하나에 보고 하나다
  ([계층과 순서](#계층과-순서)). 읽을 수 없는 값에는 비교가 없다.
- 이 진단들은 어느 것도 다른 쓰기를 막지 않는다. 그러므로 엔진 밖의 상태 변경은 사람이 일관된 이력을 위조하지
  않는 한 보고된다.
- **Commit.** commit은 먼저 그 대상이 읽은 뒤 바뀌지 않았는지 검사한다: 바뀌었으면 `WRITE_CONFLICT`다.
  이력 빈틈(`history.gap`)을 포함한 constraint 검사는 잠금 아래에서 다시 읽은 corpus 위에서 실행한다.
  다른 행의 validator 진단(`type`, `key.duplicate`, `reference.dangling`, `check`, `order`, `state.transition`, `state.required`, `state.limit`, `history.gap`, `history.chain`, 여러 `files` file에 걸친 `parent.cycle`)은 commit을 실패시키지 않되, `type` 진단은 commit의 제약 검사가 그 값을 평가해야 할 때 commit을 실패시키고, 바이트 단위로 같은 key를 가진 두 행은 그 table을 사용할 수 없게 한다([canonical format](10-canonical-format.md#diagnostics)); `format.*` 진단은 다시 쓰기 거절과 사용할 수 없는 table 상태([canonical format](10-canonical-format.ko.md#진단))가 정하는 경우에만 commit을 실패시킨다. file을 다시 써야 하는 commit — 행 자신의 file, 상세 섹션이 생기거나 사라지는 행으로의 link를 가진 file, `order`의 형제 — 은 그 file이 `format.*` 진단을 가지면 거절한다: `CONSTRAINT`이며 file과 진단을 밝히고, `format.noncanonical`이면 `flowmark canon`을 밝힌다. 세 commit은 면제된다: 대상에 대한 `flowmark canon`([operations](40-ops.ko.md#canon-연산)), 추가된 column이 일으킨 진단에 대한 add-column migration([migration](80-migration.ko.md)) — 그 진단은 그 migration에 대해 table을 사용할 수 없게 하지 않는다 —, `files` 행의 file 전체를 지우는 delete는 rewrite 거절에서만 면제되며, restrict 검사는 그 행을 참조할 수 있는 모든 table의 행을 여전히 필요로 하며(자신의 table을 `parent`로 하는 것, 자식 block과 이력 행, 그 행으로의 foreign key를 가진 모든 table), 그 중 하나라도 사용할 수 없으면 그 delete는 `CONSTRAINT`다.
- **Repair.** `flowmark history repair <table> <key> [--from <initial state>]`는 마지막 `to`에서 — 이력
  없는 행은 하나뿐인 초기 상태, 또는 초기 상태가 여럿이면 필수인 `--from`에서 — 현재 상태로 가는 이력
  행 하나를 insert한다. `at`은 엔진 시계이고 필수 column은 행에서 복사한다. 그 쌍이 선언된 전이가
  아니거나, 전이가 요구하는 column이 행에서 null이거나 비었거나, 이 행을 세어 현재 상태의 limit를 넘으면
  거절하며, 그때 사람이 편집으로 합법 상태를 되돌리거나 column을 채운다. repair는 사람이 commit하는 보통
  transaction이다.
  repair는 또한 빈틈이 없는 행 — 상태가 마지막 이력 행의 `to`이거나, 이력 행이 없으면 초기 상태이거나,
  상태가 NULL인 행 — 을 상태 column에서 `CONSTRAINT`로 거절한다. 일어나지 않은 전이의 이력 행을
  insert하면 흔적을 위조하기 때문이다. 이력 행이 있는 행에 준 `--from`(repair는 마지막 `to`에서 간다),
  초기 상태가 아닌 `--from`, 초기 상태가 여럿인 기계에서 빠진 `--from`, 이력을 가진 상태기계가 없는
  table은 `CONFIG`로 거절한다. catalog가 선언하지 않은 table은 schema에 없는 이름의 orm code인
  `ENTITY_UNKNOWN`으로 거절한다. 어떤 행도 바이트 단위로 가리키지 않는 key는 한 행 읽기가 아무 행도
  맞추지 못했을 때의 orm code인 `NO_ROWS`로 거절한다. transaction 밖에서는 lock 아래 transaction 하나로,
  열린 transaction 안에서는 다른 쓰기처럼 실행한다. 그 `at`은 repair마다 한 번 읽은 엔진 시계이므로,
  commit이 그것을 다시 실행해도 같은 행을 쓴다([operations](40-ops.ko.md#commit-protocol)).

## 사람이 쓴 편집에 대한 판정

database는 작성자 독점을 강제하지만 corpus에는 그것이 없다. 기능별로:

- **foreign key action**: 위와 같이 restrict-only다. 사람이 부모 file을 지우면 dangling 참조가 남고
  validate가 보고한다.
- **`audit`와 `immutable`**은 corpus에서 schema 오류다 — markdown에는 trigger가 없다. 상태기계의 이력이
  기록된 흔적이고, 그 빈틈, 사슬, 전이, 필수 규칙이 사람의 편집이 우회한 것을 보고한다.
- **Default**는 엔진이 insert에서 명시적으로 쓴다. non-null column이 빠진 사람이 쓴 행은 format 진단이다.

## 진단

모든 규칙은 `{file, line, column, rule, message}`를 보고하며, column은 [canonical format](10-canonical-format.ko.md#진단)이 정하는 대로다. 규칙 이름은 `catalog`, `type`,
`key.duplicate`, `reference.dangling`, `check`, `parent.cycle`, `order`, `state.transition`,
`state.required`, `state.limit`, `history.gap`, `history.chain`, `corpus.walk`
([document model](20-document-model.md)의 walk 규칙), 그리고 canonical format의 `format.*` 규칙이다: `format.noncanonical`, `format.block`,
`format.section`, `format.encoding`. 검증은 corpus 전체에 걸친다. 깨진 corpus 안의 깨끗한 file이 corpus를
유효하게 만들지 않는다. 열기, introspection, `flowmark validate`, `flowmark canon`은 행에 대한 요청이 아니므로 사용할 수 없는 table에서 결코 실패하지 않으며, canon의 링크 유도 거절은 예외다([operations](40-ops.ko.md#canon-연산)). validator는 모든 file의 `format.*`와 `parent.cycle` 진단과, `format.block`, `format.section`, `format.encoding`이나 그 행 사이의 `parent.cycle`이 없는 file 안에서 읽을 수 있는 모든 행의 validator 규칙을 보고한다. `format.block`, `format.section`, `format.encoding`이나 그 행 사이의 `parent.cycle`이 있는 file을 가진 table의 완전한 행을 필요로 하는 규칙 — 그 table로의 `reference.dangling`, `key.duplicate`, 그 안의 `order`와 `state.limit` — 은 그 table에 대해 보고하지 않으며, file 자신의 진단이 그것을 대신한다. 여러 `files` file에 걸친 validator의 `parent.cycle`은 읽을 수 있는 행 사이에서 보고한다. 순환은 그 행들 자신의 `parent` 값으로 만들어지며 읽지 못한 어떤 행도 그것을 풀 수 없기 때문이다. 바이트 단위로 같은 key 때문에만 사용할 수 없는 table은 모든 규칙이 보고된다.
