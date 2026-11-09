<!-- doc-id: spec-constraints -->
<!-- source-sha256: d08db07fdfe5293b83b5b8fdc75283e4f4bc0c307953d74c2e0f92d92dcbab26 -->
# Constraints

[English](50-constraints.md)

이 spec은 엔진이 쓰기에서 강제하는 것과 validator가 corpus 위에서 보고하는 것을 고정하며, 엔진을
우회하는 작성자 — file을 편집하는 사람 — 에 대한 판정도 포함한다. 실패 code는
[operations](40-ops.md)의 것이다.

## Parse와 type

- **Strict parse.** [문법](10-canonical-format.md)을 벗어난 줄, 블록, 섹션, key는 위치가 있는 진단이다.
  관대한 mode도, 보이지 않는 데이터도 없다.
- **Type.** 모든 값은 [canonical format](10-canonical-format.md)의 읽을 수 있는 형식으로 자기 dbspec type을 따른다. type을 어긴 값은 그 자리의
  진단이 validator의 `type`이고, 엔진이 시작하는 쓰기가 그 값을 배정하면 `CONSTRAINT`이며, type 밖의 orm parameter는 `CODEC_ENCODE`다([canonical format](10-canonical-format.md), [capability table](60-dialect.ko.md#wire)).

## Key와 참조

- **primary key와 unique key**는 table 전체에서 강제한다 — block table이면 그것을 가진 모든 file의 모든 block에 걸쳐서. 중복은 그것을 새로 만드는 쓰기에서 `DUPLICATE_KEY`이고, validator의 `key.duplicate`는 [corpus 순서](10-canonical-format.ko.md#진단)에서 첫 자리 다음의 각 자리에, 그 key에 있으며 첫 자리를 밝힌다. unique key는 SQL처럼 NULL을 서로 다르게 본다. unique key의 validator `key.duplicate`는 값을 각각 정식 readable 형식으로 바이트 단위로 비교하며, corpus 순서에서 첫 자리 다음의 각 자리에, key의 첫 column에 있고 첫 자리를 밝힌다. 그 column 중 어느 것의 값이 NULL이거나 column의 type이 아닌 행은 비교에 참여하지 않는다. 근거: SQL과 같다 — orm은 모든 문자 column에 code point를 비교하는 binary collation 하나(`utf8mb4_0900_bin`, `COLLATE "C"`, `BINARY`)를 주고 NULL의 반복을 허용한다 — 그리고 엔진이 읽지 못하는 값에는 비교가 없으며 그 `type`이 그것을 대신한다. primary key가 대소문자를 무시하고 비교하는 것은 그것이 file 이름이자 anchor이기 때문일 뿐이다([document model](20-document-model.ko.md#key)). unique key의 값은 어느 쪽도 아니므로 그 이유가 미치지 않는다.
- **foreign key**는 참조된 key와 byte 단위로 정확히 일치해야 풀린다. 어떤 행도 가리키지 않는 참조는
  dangling 참조다: 그것을 새로 만드는 쓰기는 `FOREIGN_KEY`이고, validator의 `reference.dangling`이 참조 자리에 보고하며, file을 읽을 수 없게 하지 않는다. 다만 block이나 document 행의 `parent`가 그 file의 어떤 행도 가리키지 않으면 `format.block`이다([canonical format](10-canonical-format.md#foreign-keys-as-links)).
- **key는 결코 갱신되지 않는다.** primary key를 배정하는 update, 그리고 primary key에 새 값을 쓰는 restore(restore가 고른 key 밖의 key column 값)는 `IR_INVALID`(모든
  database에서 orm의 규칙)이므로 link의 target은 link 아래에서 움직이지 않는다. key는 새 key로 행을
  insert하고, 참조하는 행이 그것을 가리키게 하고, 옛 행을 지우는 것으로 한 transaction에서 바뀐다.
- **참조 action.** `on delete cascade`와 `on delete set_null`은 행을 지우는 쓰기 안에서 엔진이 적용한다:
  cascade는 참조하는 행을 단계마다 함께 지우고, set_null은 참조하는 column을 비우며, 모두 한 commit이다.
  affected 수는 SQL처럼 요청이 맞춘 행의 수다. 사람이 부모 파일을 지우면 action은 적용되지 않고 validate가
  끊어진 참조를 보고한다.
  `restrict` foreign key는 거절한다: 참조되는 행의 삭제는
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
- 바이트 그대로 쓸 수 없는 text나 `markdown` 값, 그리고 `CanonicalLiteral`이 거절하는 typed 값(scale보다 많은 소수 자릿수 — 0 포함, precision − scale보다 많은 정수 자릿수, precision보다 많은 소수 자릿수의 time이나 datetime). orm parameter이면 type 밖의 typed 값은 U+0000을 포함해 대신 `CODEC_ENCODE`다([canonical format](10-canonical-format.md));
- `format.*` 진단이 있는 file을 다시 쓰는 logical write는 file과 진단을 밝히며 `CONSTRAINT`다. 그 진단이 `format.noncanonical`이면 `flowmark canon`을 밝힌다(commit의 면제를 제외하고)([canonical format](10-canonical-format.md)). 쓰기는 문서 바이트를 정규화하지 않는다;
- CommonMark block 구조에서 최상위 level ≤ L heading을 가진 `markdown` 값([canonical format](10-canonical-format.md));
- 상세 anchor와의 충돌(heading 또는 HTML `id`나 `name`)을 새로 만드는 쓰기 — 제목 갱신, payload 쓰기, 행 삽입, 이동,
  상세 섹션의 생성 — 는 anchor를 차지하는 heading이나 id를 쓴 행과 column — `markdown` column의 값, `files` 행의 제목(H1에 대해), 또는 section이나 block heading에 대해서는 그것이 여는 행 scope의 행, 그 heading이 이름 붙인 `markdown` column이나 자식 table — 에 위치하며, 차지한 anchor의 상세 heading을 밝히는 `CONSTRAINT`다. 다만 중복 key는 `DUPLICATE_KEY`다([canonical format](10-canonical-format.md#detail-headings-and-anchors));
- date, time 또는 datetime 값으로 쓴 `now`(그 type이 아니다), 그리고 column 길이를 넘는 text(orm parameter이면 `CODEC_ENCODE`)([canonical format](10-canonical-format.md));
- list 제목 token `<declared column>\:`([canonical format](10-canonical-format.md#list-rows));
- 최상위 setext heading을 담거나, 뒤따르는 요소가 그 안으로 흡수되는 fence나 HTML block을 열린 채로 둔 `markdown` 값. 그 요소를 어느 쓰기가 놓든 마찬가지다([canonical format](10-canonical-format.md#sections-and-their-values)).

읽기에서는 walk가 `corpus.walk`를, 형식 규칙이 `format.*`를 보고한다. 그 내용은
[document model](20-document-model.md)과 [canonical format](10-canonical-format.md)이 정한다.

## Check

check는 commit에서 강제한다. 술어가 unknown(NULL)인 check는 SQL처럼 통과한다. validator는 table의 각 check를 읽을 수 있는 각 행에 대해 그 행의 논리 값 — 읽기가 돌려주는 대로의 모든 column, 그리고 부모 행의 key인 배치 foreign key — 으로 평가하고, 술어가 false이면 행의 첫 줄에서 check와 행을 밝혀 `check`를 보고한다. text와 `uuid`는 code point로(orm의 binary collation 하나), 수는 값으로, date, time, datetime은 시간순으로 비교한다. column의 type이 아닌 값은 `is [not] null`에서 NULL이 아니며, 그 값을 읽고 NULL operand가 없는 비교나 `in`은 한 번은 true로, 한 번은 false로 두 번 평가한다: dbspec 술어에는 `not`이 없으므로 이 atom들에 대해 단조이고, 첫 평가는 어떤 값이 줄 수 있는 가장 큰 결과, 둘째는 가장 작은 결과다. validator는 첫 평가가 false이면 — 그 값이 무엇이든 check가 false이면 — `check`를 보고하고, 나머지는 그 값의 `type`이 대신한다. commit은 첫 평가가 false이면 check를 밝히는 `CONSTRAINT`, 둘째만 false이면 check가 그 값을 필요로 하므로 `type` 진단을 밝히는 `CONSTRAINT`이고, 그 밖에는 통과한다. 근거: check는 자기 행의 술어이므로 행을 읽을 수 있는 곳이면 사용할 수 없는 table에서도 보고한다. SQL은 어긴 check를 column이 아니라 그 이름으로 밝힌다.

## Codec 값

`codec` setting을 가진 column은 encode된 값을 그 type의 readable 형태로 담는다
([canonical format](10-canonical-format.ko.md#읽을-수-있는-값-형식)). validator는 type에 맞는 non-NULL 값마다
orm의 Go codec package — stage `gz`, `base64`, `serialize`, `yaml`, `ordered_json`을 가진 `orm.Decode` —
로 decode하며, `bytes` 값은 그 base64 텍스트에서 decode한다. stage가 거절하면 그 값의 요소에서 orm의
message를 실어 `codec`을 보고한다. 그런 값을 할당하는 쓰기 — insert는 모든 값을 할당한다 — 는 그
column에서 `CONSTRAINT`이고 message가 `codec`을 밝히며, commit은 validator의 decode로 그것을 판정한다.
쓰기가 그대로 두는 값은 판정하지 않는다. 그대로 두는 `type` 결함이 check가 그것을 읽을 때에만 쓰기를
실패시키는 것과 같다. column의 type이 아닌 값은
`type`만이다. stage `aes`가 있는 column은 행이 기록한 version의 key로 orm의 `HostDecode`가 decode한다([schema](30-schema.md#keys)). keyring이 선언하지 않은 version은 `codec`이고, keyring이 없는 process에서는 그 column을 판정하지 않는다. 그 밖에 host stage `hex`나 `ip`를 포함하는 column은 orm이 host decode를 공개할 때까지(orm `T74`)
판정하지 않는다: `orm.Decode`는 host stage를 담은 모든 목록을 거절하므로, 판정하면 모든 값을 보고하게
된다. 그때 목록 전체를 거꾸로 decode하며, host stage는 orm의 `HostDecode`로, 나머지는 `orm.Decode`로
한다. 근거: 자기 stage가 decode할 수 없는 값은 그것을 읽는 어떤 client에게도 답이 없으므로, validator가
밝히는 보이지 않는 데이터다. decode는 orm의 것이다. 값을 읽는 client가 그것으로 decode하기 때문이며,
엔진은 어떤 stage도 다시 구현하지 않는다.

## View

view table은 응용 schema의 table `views`다([응용 schema](100-checklist.ko.md#view)). 그 행 하나하나가
`kind`, `entity`, `filter`, `sort`, `group_by`를 가진 view다. validator는 읽을 수 있는 모든 행을 판정한다:

- **Entity.** `entity`는 catalog에 대한 orm runtime model의 entity를 이름 붙인다. `kind`가 `kanban`,
  `list`, `gantt`이면 그 entity의 table은 `title`과 `body`를 선언한다
  ([document model](20-document-model.ko.md#제목과-body)). `kind`가 그 type의 값이 아니면 `entity`는
  entity를 이름 붙이는지만 판정한다.
- **Filter.** `filter`는 정확히 member `where`(IR `Group`)와 `params`(항목이 null, boolean, 수, 문자열인
  배열)를 가진 JSON object 하나를 담는다. filter 없음은 `{"where":{"items":[]},"params":[]}`다. validator는
  request `{ir_version, model의 manifest_hash, kind all, entity, where, n_params}`(n_params는 param의 수)에
  orm의 `ir.Validate`를 실행하고 그것이 돌려준 code를 보고한다. 어떤 `p`나 `ps` index도 쓰지 않는 param은
  결함이다.
- **Sort.** `sort`는 IR `Order`의 JSON 배열을 담으며 `[]`는 정렬 없음이다. `{…, kind all, entity, order,
  n_params 0}`에 `ir.Validate`로 검증한다.
- **Group.** `group_by`는 grouping 없음이면 NULL이고, 아니면 `ir.OpAllowed(field, "eq")`가 허용하는
  entity의 field를 이름 붙인다.
- **모양.** 어느 깊이에서든 반복된 object member, 모르거나 빠진 member, 정수가 아닌 index, 그 밖의
  다른 모양의 `filter`나 `sort`는 잘못된 형식이다.

결함은 각각 그 column의 요소에 있는 `view`이며 column마다 보고 하나다. `entity`에 결함이 있으면
`filter`, `sort`, `group_by`는 판정하지 않는다. view column의 `type`, `codec`, `check` 결함은 그것만으로
서며, `kind`는 자기 `view`를 갖지 않는다. 이 규칙은 자기 행만 필요로 하므로 사용할 수 없는 table에서도
보고한다. catalog 변경으로 유효하지 않게 된 view는 보고하며 읽기에서 거절하지 않는다. view를 유효하지
않게 남기는 쓰기는 `CONSTRAINT`다: commit은 쓰기가 어느 column을 할당하든 자기가 쓰는 view 행마다
validator의 판정으로 판정하며, 결함은 각각 그 column에 있고 message가 `view`를 밝힌다. 그래서 catalog
변경으로 유효하지 않게 된 view는 유효하게만 쓰인다. validator는 param을 그 쓰임의 type에 대해 판정하지 않는다: 그 type은
orm의 planner가 계산하고 공개하지 않은 slot type이며(orm `T89`), executor의 parameter 검사도 같은 원천을
기다린다. 다른 type의 param은 view를 실행하는 읽기가 거절하며, orm이 slot을 공개하면 validator는 각
param을 모든 쓰임에 대해 판정한다. `ir.Validate`가 검사하는 것은 orm의 계약이다. 그 빈틈 — 해석하지
않는 column 대 column `ref`와 group 첫 항목의 `conn`(orm `T73`) — 은 orm에서 닫힌다. 근거: view는 저장된
request이므로 request를 판정하는 규칙이 현재 catalog에 대해 그것을 판정한다 — view는 데이터이고 catalog
변경이 그것을 보이지 않게 고아로 만들면 안 되므로 manifest hash를 저장하지 않는다. 어떤 index도 쓰지 않는
param은 보이지 않는 데이터다. grouping은 같은 값을 가진 행을 모으므로, 같음이 없는 field — codec이
정확히 `ip`가 아닌 codec field — 에는 group이 없다.

## 계층과 순서

- `parent` 순환은 쓰기에서 `CONSTRAINT`이고, 한 file의 행 사이의 순환은 읽기에서 `parent.cycle`이며 그 file을 읽을 수 없게 한다([document model](20-document-model.ko.md#계층)); 여러 `files` file의 행에 걸친 순환은 validator의 `parent.cycle`뿐이다. 행의 블록 밖 `parent`는 쓰기에서 `CONSTRAINT`, 읽기에서
  `format.block`이다.
- 1..n + 1 밖의 순서 값(n은 문서 모델이 정의하는 대로)이나, 둘 이상의 행과 맞는 order column update는
  `CONSTRAINT`다. block과 document 저장에서 쓰기 뒤의 형제 묶음 순서 값이 정확히 1..m(m은 그 묶음의 쓰기 뒤 크기)이 아니면 `CONSTRAINT`다. `files` 저장에서 order 항목은 rank를 담으며, validator의 `order`는 rank를 형제와 공유하는 각 행에, 그 `order` 항목에 있다 — rank 사이의 빈틈은 위치를 바꾸지 않으므로 결함이 아니다 — 그리고 column의 type이 아닌 rank는 `type`뿐이며 어느 것과도 공유되지 않는다([document model](20-document-model.ko.md#순서)). 근거: 읽을 수 없는 값에는 비교가 없으므로 type 결함이 그것을 대신하며, 그 값이 무엇이든 다른 형제가 가지는 결함은 여전히 보고된다.
- `parent`가 그 column의 type이 아닌 행은 알려진 형제 묶음에 속하지 않는다: 자신의 `order`는 판정하지 않으며 그 `type`이 대신한다. 묶음 안에서 공유되는 rank는 그런 행이 어느 묶음에 들어가든 보고한다.

## 상태 column

- **상태기계 없음.** database는 상태기계를 갖지 않는다: column이 담을 수 있는 값은 그 type과 schema의 제약이며,
  어떤 database도 update를 그것이 대신하는 값 때문에 거절하지 않는다. catalog는 orm 문법의 `state_machine` 줄 —
  `initial`, 전이, `terminal`, `require`, `history`, `limit` — 을 읽고, 엔진은 그 어느 것도 적용하지 않는다. orm
  DBS-73이 database가 그렇게 한다고 말하는 대로이며, MySQL, PostgreSQL, SQLite가 하는 대로다: 어떤 insert도 그 상태
  때문에, 어떤 update도 그것이 대신하는 상태나 종료 상태를 떠나는 것 때문에 거절하지 않고, 어떤 `require` column도
  강제하지 않으며, 어떤 `limit`도 세지 않고, 어떤 이력 행도 쓰지 않으며, validator는 기계의 어떤 규칙도 보고하지
  않는다. `history` 줄이 이름 붙인 table은 보통 table이다. 상태 column은 모든 database에서처럼 schema의 제약이 허용하는
  그 type의 값을 담는다: check `in (…)`은 다른 값을 다른 check처럼 거절하고([Check](#check)), 상태 table로의 foreign
  key는 어떤 행도 가리키지 않는 값을 다른 foreign key처럼 거절한다([Key와 참조](#key와-참조)). 근거: orm의 규칙 하나는
  모든 database에서 같은 결과이므로 엔진은 그 database들이 가진 것만 가진다. 상태의 전이와 그 흔적은 polyspec fsm의
  것이며, fsm이 취한 상태는 다른 값처럼 corpus에 쓰인다.
- **Checkbox.** list로 저장된 table에서 그 table의 `checkbox` 줄이 이름 붙인 column은 list 행의 괄호 사이에 행 값의
  glyph로 쓰인다([canonical format](10-canonical-format.ko.md#list-행)). 어떤 `checkbox` 줄도 이름 붙이지 않은 값의
  list 쓰기는 list가 그것을 보일 수 없으므로 `CONSTRAINT`이고, 읽기에서 어떤 줄도 이름 붙이지 않은 glyph는
  `format.block`이다. `checkbox`를 선언하지 않은 list table은 그 column을 field로 쓰며, `table`과 `files` 저장에서 그
  column은 다른 scalar처럼 cell이나 field 항목이다. `checkbox` 줄의 형태는 [schema](30-schema.ko.md#markdown-setting)가
  정한다.

## Commit

- commit은 먼저 그 대상이 읽은 뒤 바뀌지 않았는지 검사한다: 바뀌었으면 `WRITE_CONFLICT`다. constraint 검사는 그 다음
  잠금 아래에서 다시 읽은 corpus 위에서 실행한다.
- 다른 행의 validator 진단(`type`, `key.duplicate`, `reference.dangling`, `check`, `codec`, `view`, `order`, 여러 `files` file에 걸친 `parent.cycle`)은 commit을 실패시키지 않되, `type` 진단은 commit의 제약 검사가 그 값을 평가해야 할 때 commit을 실패시키고, 바이트 단위로 같은 key를 가진 두 행은 그 table을 사용할 수 없게 한다([canonical format](10-canonical-format.md#diagnostics)); `format.*` 진단은 다시 쓰기 거절과 사용할 수 없는 table 상태([canonical format](10-canonical-format.ko.md#진단))가 정하는 경우에만 commit을 실패시킨다.
- file을 다시 써야 하는 commit — 행 자신의 file, 유도된 target이 바뀌는 행(상세 섹션이 생기거나 사라지거나, insert가 dangling link에 그 행을 주는 행)으로의 link를 가진 file, `order`의 형제 — 은 그 file이 `format.*` 진단을 가지면 거절한다: `CONSTRAINT`이며 file과 진단을 밝히고, `format.noncanonical`이면 `flowmark canon`을 밝힌다. 세 commit은 면제된다: 대상에 대한 `flowmark canon`([operations](40-ops.ko.md#canon-연산)), 추가된 column이 일으킨 진단에 대한 add-column migration([migration](80-migration.ko.md)) — 그 진단은 그 migration에 대해 table을 사용할 수 없게 하지 않는다 —, `files` 행의 file 전체를 지우는 delete는 rewrite 거절에서만 면제되며, restrict 검사는 그 행을 참조할 수 있는 모든 table의 행을 여전히 필요로 하며(자신의 table을 `parent`로 하는 것, 자식 block, 그 행으로의 foreign key를 가진 모든 table), 그 중 하나라도 사용할 수 없으면 그 delete는 `CONSTRAINT`다.

## Audit

`audit` setting을 가진 table은 database의 audit trigger가 감사하듯 감사된다(orm dbspec DBS-93). 엔진이 그 table의
행에 실행하는 모든 insert, update, soft delete는 setting의 이력 table에 행 하나를 쓴다: action(`insert`, 또는 update와
soft delete에는 `update`), 이전 audit 기록, 행의 기록되는 각 column을 담는다. setting이 `tables` table을 이름 붙이면,
audit 기록에 대해 그 table을 이름 붙이는 그 table의 행 하나도 쓰며, 그 행이 이미 있으면 쓰지 않는다. 쓰인 행의 audit
column은 요청의 audit 기록을 담으며, 요청 message가 그것을 `audit` member로 나른다. 그것이 없는 audited table의 쓰기는
`CONFIG`다. audited table의 행은 그 soft delete column으로 지우며, 그 밖의 delete는 `CONSTRAINT`다. `rows.load`는 행을
저장된 형태로 쓰고 아무것도 기록하지 않으며, SQL statement의 차이는 그 database의 trigger가 쓴 행을 이미 담고
있으므로([operations](40-ops.ko.md#sql-statements)) 엔진은 아무것도 더하지 않는다. 근거: audit은 database가 자기가
실행하는 쓰기마다 쓰는 행이므로, 엔진은 database처럼 자기가 실행하는 쓰기에 대해 그것을 쓴다.

## 사람이 쓴 편집에 대한 판정

database는 작성자 독점을 강제하지만 corpus에는 그것이 없다. 기능별로:

- **foreign key action**: 위와 같이 엔진이 실행하는 쓰기에 적용한다. 사람이 부모 file을 지우면 dangling 참조가 남고
  validate가 보고한다.
- **`immutable`**: 엔진을 통한 그 table 행의 update와 delete는 `CONSTRAINT`(`table <name> is immutable`)이고,
  insert는 받아들이며, 사람이 파일을 고치는 것은 파일 시스템이 허용하는 일이다.
- **`audit`**: 엔진은 자기가 실행하는 쓰기의 audit을 쓴다([Audit](#audit)). 사람이 file을 고치는 것은 audit 행을
  쓰지 않으며 파일 시스템이 허용하는 일이다.
- **Default**는 엔진이 insert에서 명시적으로 쓴다. non-null column이 빠진 사람이 쓴 행은 format 진단이다.

## 진단

모든 규칙은 `{file, line, column, rule, message}`를 보고하며, column은 [canonical format](10-canonical-format.ko.md#진단)이 정하는 대로다. 규칙 이름은 `catalog`, `type`,
`key.duplicate`, `reference.dangling`, `check`, `codec`, `view`, `parent.cycle`, `order`, `corpus.walk`
([document model](20-document-model.md)의 walk 규칙), 그리고 canonical format의 `format.*` 규칙이다: `format.noncanonical`, `format.block`,
`format.section`, `format.encoding`. 검증은 corpus 전체에 걸친다. 깨진 corpus 안의 깨끗한 file이 corpus를
유효하게 만들지 않는다. 열기, introspection, `flowmark validate`, `flowmark canon`은 행에 대한 요청이 아니므로 사용할 수 없는 table에서 결코 실패하지 않으며, canon의 링크 유도 거절은 예외다([operations](40-ops.ko.md#canon-연산)). validator는 모든 file의 `format.*`와 `parent.cycle` 진단과, `format.block`, `format.section`, `format.encoding`이나 그 행 사이의 `parent.cycle`이 없는 file 안에서 읽을 수 있는 모든 행의 validator 규칙을 보고한다. `format.block`, `format.section`, `format.encoding`이나 그 행 사이의 `parent.cycle`이 있는 file을 가진 table의 완전한 행을 필요로 하는 규칙 — 그 table로의 `reference.dangling`, `key.duplicate`, 그 안의 `order` — 은 그 table에 대해 보고하지 않으며, file 자신의 진단이 그것을 대신한다. 여러 `files` file에 걸친 validator의 `parent.cycle`은 읽을 수 있는 행 사이에서 보고한다. 순환은 그 행들 자신의 `parent` 값으로 만들어지며 읽지 못한 어떤 행도 그것을 풀 수 없기 때문이다. 바이트 단위로 같은 key 때문에만 사용할 수 없는 table은 모든 규칙이 보고된다.
