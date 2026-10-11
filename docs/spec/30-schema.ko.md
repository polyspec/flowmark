<!-- doc-id: spec-schema -->
<!-- source-sha256: 96c0d67bcfaab53edaab705276857f61c90b094d1d4b3db6b80716561ee11cb4 -->
# Schema

[English](30-schema.md)

이 spec은 corpus의 schema를 고정한다: catalog, markdown setting, dbspec 구성에 대한 corpus 규칙, 이름
규칙, 그리고 catalog의 두 operation — install과 introspection.

## Catalog

- corpus root의 `dbspec/*.dbs` 문서 set이 catalog다: database catalog처럼 schema의 유일한 출발점이다.
  문서는 schema 사본도 stamp도 갖지 않는다 — database는 schema를 행마다가 아니라 한 번만 두며, 문서마다
  사본을 두면 사본의 이탈이 그 자체로 버그 부류가 된다.
- flowmark는 orm의 Go `engine/dbspec` package로 catalog를 읽고 그 manifest hash를 쓴다. 문법을 다시
  구현하지 않는다. `use` 줄은 `dbspec/` set 안에서 풀리며 외부 문서는 schema 오류다.
- `dbspec/`에는 `.dbs` 집합만 있다. `dbspec/`의 다른 file은 `catalog` 진단이다.
- corpus setting file `dbspec/flowmark.json`과 그 `translations` 언어 목록은 철회된다: catalog가 모든
  선언을 담으며, `.dbs` 집합 밖의 file은 읽지 않는다.
- 내장 schema fragment를 가진 단독 문서는 없다: corpus는 catalog를 가진 root다.

## Markdown setting

markdown 전용 선언은 모두 dbspec **manifest setting**이며 `markdown <column>` setting을 선례로 따른다:
SQL backend는 무시하고, database의 introspection은 일반 column을 돌려주며, setting은 자신이 적용되는
column을 이름 붙인다 — column 이름이 동작을 고르지 않는다.

| Setting | 뜻 |
| --- | --- |
| `store files` / `store document list\|table` / `store block <fk> list\|table` | table의 저장([document model](20-document-model.md)). corpus에서 table마다 정확히 하나 |
| `key_prefix <p>` | `varchar` primary key를 `<p><n + 1>`로 할당한다. orm은 이 column을 executor가 할당하는 것으로 다룬다 — `varchar`다. dbspec은 `text` column을 key로 삼지 않기 때문이다 |
| `title <column>` | 행의 제목: non-null, 비지 않음 |
| `body <column>` | 행의 body: `markdown` column, NULL이 아님 |
| `order <column>` | 형제 사이에서 행의 위치. orm은 이 column을 executor가 할당하는 것으로 다룬다 |
| `checkbox <column> <state> '<glyph>'` | 상태의 list glyph: 정확히 한 글자이며 list 행의 대괄호 사이(`- [<glyph>] …`)에 쓴다 |
| `markdown <column>` | column이 markdown prose를 담고 섹션으로 저장된다 |
| `state_machine <column> initial <state>` | 초기 상태. 반복할 수 있다 |
| `state_machine <column> <from> -> <to> [require (…)]`, `state_machine <column> terminal <state> [require (…)]` | dbspec이 정의하는 전이와 종료 상태 |
| `state_machine <column> history <table> row <fk column> from <column> to <column> at <column>` | 이력 table과 이름 붙인 그 column |
| `state_machine <column> limit <state> <n>` | 그 상태의 행은 최대 n개 |

## dbspec 구성에 대한 corpus 규칙

- **거절**: corpus에서 각각 schema 오류다 — composite primary key, `aes`·`aes_version`·`blind_index`
  (key는 client를 떠나지 않는다), `audit`와 `immutable`(markdown에는 trigger가 없고 대신
  `state_machine … history`가 전이를 기록한다), `cascade`와 `set_null` action, 외부 `use` 문서, `store`
  setting 없는 table.
- **허용**: markdown table의 `i64` primary key column에만 `identity`를 쓴다. lock 아래 `max+1`로 할당한다. dbspec은 그 밖의 type에 대한 `identity`를 거절하므로(`an identity column has type i64`), `i16`이나 `i32` key도 쓸 수 없다. 근거: `max+1`은 정수 순열이며, `bool`, `varchar`, `uuid`, `date` key에는 더할 최댓값이 없다.
- **요구**: `state_machine`을 가진 table은 `history`와 적어도 하나의 `initial` 상태를 선언한다. `list`나
  `files` 저장은 `title`을 선언한다. state machine을 가진 `list` 저장은 `checkbox`를 적어도 하나 선언하며, dbspec이 그 뒤로 모든 상태를 덮도록 요구한다. dbspec은 checkbox가 하나도 없는 state machine을 받아들이므로 이 규칙은 flowmark의 것이다.
- **key 타입.** markdown table의 단일 column primary key는 integer type(`i16`, `i32`, `i64`), `bool`, `varchar`,
  `uuid`, `date` 중 하나다. 다른 type은 schema 오류이며 table 이름에 위치한다. 근거: key는 file 이름이자 anchor이므로
  그 텍스트는 key 모양의 문자만 가져야 한다([document model](20-document-model.md#keys)). `date`는 연도가 0001부터
  9999까지인 `YYYY-MM-DD`로 쓰며(orm의 date literal 규칙이 받는 범위), 그래서 네 자리 숫자이고 부호가 없으며 `-`는 구분자일 뿐이다. `time`과 `datetime`의 보통 값은 `:`를 담는다. scale이 0보다 큰 `decimal`과 `f64`의 정수가 아닌 값은 `.`을 담으며, 이 type들의 음수 값은 `-`로 시작하므로 이들은 모양을 결코 만족할 수 없다. scale이 0인 `decimal`은 정수만 담으며 정수 key는 integer type이 맡으므로, 다른 decimal과 함께 거절한다. `f64`의 정수 값도 integer type이 맡는 정수이므로, `f64` key는 모양이 거절하는 값만 더한다. `text`와 `bytes`는 dbspec 자신의 key 규칙이 거절한다. 음수 integer key는 type이 아니라
  값마다 거절된다([document model](20-document-model.md#keys)).
- **외래 key.** table의 모든 외래 key는 하나의 column이며 어떤 table의 단일 column primary key를 가리킨다.
  composite 외래 key, 또는 unique이지만 key가 아닌 column을 가리키는 외래 key는 그 외래 key의 위치에 놓인
  schema 오류다. 근거: 외래 key는 가리키는 행으로의 link로 쓰이며, 행은 단일 column key를 통해서만 link를 가진다.
- **block 배치.** block table의 배치 외래 key — 그 `store block <fk>` setting이 이름 붙이는 외래 key — 의
  column은 non-null이다. nullable이면 block table 이름에 위치한 schema 오류다. 근거: block 행은 그 배치
  외래 key가 참조하는 행의 scope 안에 서며([document model](20-document-model.md#storage)) NULL은 어떤 행도
  참조하지 않으므로, 그 자리에 NULL을 가진 행은 쓰일 자리가 없다. SQL 행은 자리가 필요 없으므로 dbspec은 nullable
  외래 key column을 받아들인다. 이 catalog 규칙은 여기서 정한다. 이 규칙은 이력 table을 포함한 모든 block
  table에 성립한다([이력 table](#이력-table)).

## 이름 규칙

- **예약된 table 이름.** 대소문자를 무시하고 `CON`, `PRN`, `AUX`, `NUL`, `COM0`–`COM9`, `LPT0`–`LPT9`와
  같은 table 이름은 schema 오류다: Windows의 예약된 file 이름이기 때문이다.
- **부모의 markdown column과 같은 이름을 가진 자식 table**은 schema 오류다.
- **heading 깊이.** layout이 H6를 넘는 heading을 필요로 하면 schema 오류이며, catalog를 읽을 때 정해진다. table의 행은 body가 아닌 markdown column이나 child block을 가질 수 있으면 자기 row scope 수준에 section을 쓰고, body를 가질 수 있으면 그 한 단계 위에 detail heading을 쓴다. 둘 중 어느 것도 가질 수 없는 table은 block heading 아래에 heading을 쓰지 않으므로, row scope가 H6를 넘어도 된다. 근거: layout이 쓰지 않는 heading은 layout이 필요로 하는 heading이 아니다. markdown column이 없는 state machine의 history table은, 기계가 어떤 column도 require하지 않거나 두 table 어느 쪽도 `markdown`으로 선언하지 않은 column만 require할 때처럼, detail section이 없으므로 block row 아래 level 7에 선다.

## 상태 집합

- 기계의 상태는 `initial`, 전이, `terminal` 줄이 이름 붙인 상태들이다. table이 `checkbox`를 선언하면 `checkbox`는 서로 다른 glyph로 그
  집합을 정확히 덮는다. 다른 상태를 이름 붙인 `checkbox`, `limit`, `initial`은 setting 오류다.
- 상태 column의 `default`는 초기 상태여야 하며 아니면 setting 오류다. 종료 상태이기도 한 초기 상태는
  setting 오류다.
- table의 상태 column은 `state_machine` setting이 이름 붙이는 column이다. 모든 storage에서 non-null이며, dbspec이 state machine의 nullable 상태 column을 거절한다. 근거: 행의 상태는 기계 안의 자리이며 NULL은 자리를 가리키지 않는다.

## 이력 table

history setting은 이력 table의 column을 이름 붙인다: `<fk column>`은 선행 index를 가진 상태 table로의
foreign key를 갖고, `from`과 `to`는 상태 column의 type, `at`은 `datetime(6)`이며, `require` 목록이 이름
붙인 column마다 그 이름과 type을 가진 nullable column 하나가 있다. table은 `identity` key 외에 다른
column이 없고 `title`이나 `body`를 선언하지 않는다. 어긋나면 이력 table 이름에서 setting 오류다.
상태 table 아래의 `block <fk> table`로, 또는 `document table`로 저장된다. 이력 table의 다른 저장은 이력 table 이름에서 setting 오류다. 근거: 이력 규칙 — 전이마다 행 하나, gap과 chain 진단, 복구, restrict delete([constraints](50-constraints.md#the-state-machine)) — 은 행과 그 foreign key 위에서 정해지며 그 자리 위에서 정해지지 않으므로, 저장은 다른 모든 table과 같이 schema가 고른다. 다른 저장은 배제된다: `files`와 `list`는 `title`을 선언하는데 이력 table은 선언하지 않는다. 다른 table 아래의 block은 이력 행을 row column이 아닌 column으로 배치하며, 이는 방금 말한 setting 오류다 — 복사된 column으로 배치한 block이 그 한 예로, 그 column은 nullable이며 그 줄이 require하지 않는 모든 전이에서 NULL이므로 block 배치 규칙도 이를 거절한다([block 배치](#dbspec-구성에-대한-corpus-규칙)). 그리고 block 배치 규칙은 row column이 nullable인, 상태 table 아래의 block도 거절한다. 두 저장은 지역성과 깊이를 맞바꾼다. block은 행의 이력을 그 행의 상세 섹션 — `files` table이면 그 행 자신의 file — 에 두므로, 행과 그 이력을 함께 읽고 서로 다른 행의 전이는 서로 다른 자리를 건드린다. 그러나 이력을 가진 모든 행이 상세 섹션을 가지며, block은 상태 행보다 두 단계 아래에 선다. document table은 table의 이력을 `<history>.flowmark.md`에 두며, 그 행은 상태 table의 깊이와 상관없이 row scope 3을 가지므로 require된 `markdown` column은 언제나 section을 가진다. 그러나 table의 모든 전이가 그 한 file에 덧붙는다. 어느 비용도 모든 corpus에 성립하지 않으므로 어느 저장도 강제하지 않는다.

`require` 목록이 이름 붙인 column은 상태 table에서 `markdown`으로 선언될 때에만 이력 table에서도 `markdown`으로 선언되며, 그 밖의 경우에는 이력 table에서 `markdown`으로 선언되지 않는다. 어긋나면 이력 table 이름에서 schema 오류다. 근거: 이력 행은 require된 column의 사본을 담고([constraints](50-constraints.md#the-state-machine)), section과 한 줄 자리는 서로 다른 값을 받는다 — 여러 줄 값이나 raw HTML은 section의 본문이지만 cell에서는 거절되고, `# x` 줄은 cell의 본문이지만 section에서는 거절된다([canonical format](10-canonical-format.md#single-line-places)) — 그래서 어긋나면 상태 table이 담은 값 때문에 선언된 전이를 거절하게 된다. dbspec은 SQL backend가 `markdown`을 무시하므로 그 어긋남을 받아들이며, catalog 규칙은 여기서 정한다. 이력 table이 block으로 저장되면 그 행은 상태 table의 행보다 두 단계 아래에 서므로, 그 row scope는 상태 table의 row scope에 2를 더한 값이다. require된 `markdown` column은 그 scope의 이력 행에 있는 section이며, 상태 table의 row scope가 5 이상이면 그 section은 H6를 넘어 서므로 layout은 이력 table 이름에서 schema 오류다([heading 깊이](30-schema.md#name-rules)). `document table`로 저장되면 이력 행은 상태 table의 깊이와 상관없이 row scope 3을 가지므로 그런 상태기계는 load된다. 두 table 어느 쪽도 `markdown`으로 선언하지 않은 require된 column은 cell이며 heading이 필요 없고 어떤 깊이에서도 허용된다. 근거: `markdown` 값은 모든 저장에서 section이며([canonical format](10-canonical-format.md#sections-and-their-values)), CommonMark에는 H6 너머의 heading이 없어서 그런 값을 쓸 자리가 없다. 값을 cell로 쓰면 table의 깊이에 따라 column의 형태가 바뀌고, cell은 line feed를 담을 수 없다. section을 H6에 쓰면 이력 행의 상세 heading과 같은 수준에 서게 되는데, 그 수준에서 `<table> row <key>`가 아닌 heading은 row scope를 끝내며 그 내용은 어느 행에도 속하지 않는다([canonical format](10-canonical-format.md#detail-headings-and-anchors)).

## Install

`flowmark install <corpus>`는 catalog에서 저장 뼈대 — 각 `files` table의 폴더와 각 `document` table의
file `<table>.flowmark.md`이며, 빈 블록은 쓰이지 않으므로 블록 섹션은 없다 — 를 `CREATE TABLE`이 저장소를
만들듯 만든다. 또한 `<corpus>/.gitattributes`에 `*.flowmark.md text eol=lf` 줄을 더하며, 파일이 없으면 만들고 다른 줄은 유지한다. 그래서 git checkout은 모든 플랫폼에서 LF를 유지한다([canonical format](10-canonical-format.ko.md#encoding과-줄-구조)). `markdown://`에 대한 orm의 schema install은 `CAPABILITY_UNSUPPORTED`다.

## Introspection

introspection은 모든 문서를 catalog에 대해 검증하고 catalog를 돌려준다. 실패한 문서는 위치가 있는
진단이다. catalog 없는 corpus에는 돌려줄 schema가 없으며, catalog 초안을 만드는 도구는 이 spec에 속하지
않는다.
