<!-- doc-id: spec-schema -->
<!-- source-sha256: d228dd93acfd27f5323d2f5225e4dd16bff60475094379695aefc35c9dec0c86 -->
# Schema

[English](30-schema.md)

이 spec은 corpus의 schema를 고정한다: catalog, markdown setting, dbspec 구성에 대한 corpus 규칙, 이름
규칙, 그리고 catalog의 두 operation — install과 introspection.

## Catalog

- orm의 set 파일(orm `docs/dbspec.md`, Document sets)인 `dbspec/dbspec-set.json`이 선언한 문서 set이 corpus
  root의 catalog다: database catalog처럼 schema의 유일한 출발점이다.
  문서는 schema 사본도 stamp도 갖지 않는다 — database는 schema를 행마다가 아니라 한 번만 두며, 문서마다
  사본을 두면 사본의 이탈이 그 자체로 버그 부류가 된다.
- flowmark는 orm의 Go `engine/dbspec` package로 catalog를 읽고 그 manifest hash를 쓴다. 문법을 다시
  구현하지 않는다. `use` 줄은 `dbspec/` set 안에서 풀리며 외부 문서는 schema 오류다.
- `dbspec/`에는 set 파일과 그것이 나열한 `.dbs` 문서가 있고, 각 문서는 `dbspec/` 바로 아래에 있다. flowmark는 set
  파일을 orm의 set reader로 읽으므로 그 규칙은 orm의 것이고 그 오류는 set 파일에 위치한 `catalog` 진단이다. set의
  모든 선언을 검사하며 아무것도 버리지 않는다:
  - `dbspec/dbspec-set.json`이 없는 corpus는 `catalog` 진단 `no dbspec/dbspec-set.json`이다;
  - set 파일이 나열하지 않은 `dbspec/`의 `.dbs` 파일은 그 이름을 적은 `catalog` 진단이다;
  - `dbspec/` 밖이나 그 하위 폴더에 있는 나열된 문서는 그 경로를 적은 `catalog` 진단이다;
  - `external` 항목은 `catalog` 진단이다: catalog는 set 밖의 문서를 쓰지 않는다;
  - `dbspec/`의 다른 file은 `catalog` 진단이다.
  근거: orm의 생성기와 language server가 같은 set 파일을 읽으므로, 선언 하나가 모든 도구의 set을 정한다. 그 옆의
  directory 형태는 따로 어긋나는 두 번째 선언이 된다.
- corpus setting file `dbspec/flowmark.json`과 그 `translations` 언어 목록은 철회된다: catalog가 모든
  선언을 담으며, set 파일과 그 문서 밖의 file은 읽지 않는다.
- 내장 schema fragment를 가진 단독 문서는 없다: corpus는 catalog를 가진 root다.

## Markdown setting

markdown 전용 선언은 모두 dbspec **manifest setting**이며 `markdown <column>` setting을 선례로 따른다:
SQL backend는 무시하고, database의 introspection은 일반 column을 돌려주며, setting은 자신이 적용되는
column을 이름 붙인다 — column 이름이 동작을 고르지 않는다.

| Setting | 뜻 |
| --- | --- |
| `store files` / `store document list\|table` / `store block <fk> list\|table` | table의 저장([document model](20-document-model.md)). corpus에서 table마다 정확히 하나. `files`나 `document` 저장은 `shard <column> <year\|month\|day>`로 끝날 수 있으며 기간마다 따로 저장한다([시간 shard](20-document-model.ko.md#시간-shard)). 절은 orm이 검사하고(non-null `date`나 `datetime` column, `order` 없음, block 아님) |
| `title <column>` | 행의 제목: non-null, 비지 않음 |
| `body <column>` | 행의 body: `markdown` column, NULL이 아님 |
| `order <column>` | column 값에 따른 형제 사이의 행 순서([document model](20-document-model.ko.md#순서)). 아무것도 할당하지 않으므로 insert가 준다(orm DBS-79) |
| `task_marker <column>` | list 행이 task marker의 괄호 사이에 쓰는 column(orm DBS-80): null이든 아니든 `varchar(1)`이나 `bool` column이며, 아래에 말하는 대로다 |
| `markdown <column>` | column이 markdown prose를 담고 섹션으로 저장된다 |

`task_marker <column>`은 table마다 한 줄이며, orm은 두 번째 줄을 반복으로, 다른 type의 column을
(`task_marker needs a varchar(1) or bool column, not <type>`), 상태별 형태 `task_marker <column> <state> '<glyph>'`를
문법 오류로 거절한다. list로 저장된 table에서 그 column은 list 행의 괄호 사이에 쓰인다,
`- [<value>] <key> <title> …`([canonical format](10-canonical-format.ko.md#list-행)): `varchar(1)` 값은 그대로, 빈
텍스트는 `[]`로; `bool`은 GFM의 체크되지 않은 상자와 체크된 상자, false는 `[ ]`, true는 `[x]`로 쓰며, GFM이 역시
체크된 것으로 읽는 `[X]`는 true로 읽고 `format.noncanonical`이다; NULL은 상자 없는 list 행 `- <key> <title> …`이다. 그
값은 모든 database에서처럼 그 type과 schema의 제약 — check `in (…)`이나 상태 table로의 foreign key — 이 허용하는
것이며, glyph의 목록이 아니다. `table`과 `files` 저장에서 그 column은 다른 scalar처럼 cell이나 field 항목이다. 예전
이름인 `checkbox`와 `state_machine`은 문법의 setting이 아니며 각각 알 수 없는 setting으로 거절된다
(`setting "checkbox" is unknown`): 엔진은 상태기계를 갖지 않는다([constraints](50-constraints.ko.md#상태-column)). 근거:
GFM task list item은 괄호 사이, 곧 그 task list item marker에 한 글자를 담으므로, 한 글자 column은 거기에 값 전체를
보이고, boolean은 GFM 자신의 두 상자로 보인다.

## dbspec 구성에 대한 corpus 규칙

- **기본 저장**: `store` setting이 없는 table은 자기 문서 하나에 `document table`로 저장한다. 그래서 SQL
  데이터베이스용으로 쓴 dbspec 문서를 그대로 corpus에 설치할 수 있다.
- **composite key**: `document`나 `block` 저장 table은 여러 column의 primary key를 가질 수 있다. key cell은 값을
  `/`로 이어 담고 heading은 column 이름을 `/`로 잇는다. 예: `| tenant/account | name |`, `| 900/1 | a |`. key type의 어떤
  값도 `/`를 담지 않으므로 cell의 읽기는 하나다. 각 값은 그 column type의 key 규칙을 따르고, 읽기와 쓰기는 그 column을
  column으로 본다. composite 외래 key는 link가 아니라 평범한 값을 담고 그 table의 key 전체를 가리키며, validator의
  `reference.dangling`이 적용된다.
- **거절**: corpus에서 각각 schema 오류다 — `files` 저장 table의 composite primary key(파일 이름은 key column 하나로 정한다), 외부 `use` 문서,
  마지막 stage가 `yaml`인 `codec` — orm의 encoder(go-yaml의 `Marshal`)는 모든 값을
  line feed로 끝내며 형식의 어떤 자리도 그것을 담지 못한다. 그 뒤의 stage는 그것을 encode해 없앤다.
  view table은 setting이 아니라 [응용 schema](100-checklist.ko.md#view)가 선언한다.
- **허용**: markdown table의 `i64` primary key column에만 `identity`를 쓴다. lock 아래에서 가장 큰 key와 표의 counter 가운데 큰 것보다 하나 큰 key로 할당한다([document model](20-document-model.ko.md#key)). dbspec은 그 밖의 type에 대한 `identity`를 거절하므로(`an identity column has type i64`), `i16`이나 `i32` key도 쓸 수 없다. 근거: `max+1`은 정수 순열이며, `bool`, `varchar`, `uuid`, `date` key에는 더할 최댓값이 없다.
- **요구**: `list`나 `files` 저장은 `title`을 선언한다.
- **key 타입.** markdown table의 단일 column primary key는 integer type(`i16`, `i32`, `i64`), `bool`, `varchar`,
  `uuid`, `date` 중 하나다. 다른 type은 schema 오류이며 table 이름에 위치한다. 근거: key는 file 이름이자 anchor이므로
  그 텍스트는 key 모양의 문자만 가져야 한다([document model](20-document-model.md#keys)). `date`는 연도가 0001부터
  9999까지인 `YYYY-MM-DD`로 쓰며(orm의 date literal 규칙이 받는 범위), 그래서 네 자리 숫자이고 부호가 없으며 `-`는 구분자일 뿐이다. `time`과 `datetime`의 보통 값은 `:`를 담는다. scale이 0보다 큰 `decimal`과 `f64`의 정수가 아닌 값은 `.`을 담으며, 이 type들의 음수 값은 `-`로 시작하므로 이들은 모양을 결코 만족할 수 없다. scale이 0인 `decimal`은 정수만 담으며 정수 key는 integer type이 맡으므로, 다른 decimal과 함께 거절한다. `f64`의 정수 값도 integer type이 맡는 정수이므로, `f64` key는 모양이 거절하는 값만 더한다. `text`와 `bytes`는 dbspec 자신의 key 규칙이 거절한다. 음수 integer key는 type이 아니라
  값마다 거절된다([document model](20-document-model.md#keys)).
- **외래 key.** table의 모든 외래 key는 어떤 table의 primary key를 가리킨다. unique이지만 key가 아닌 column을
  가리키는 외래 key는 그 외래 key의 위치에 놓인 schema 오류다. 근거: 단일 column 외래 key는 가리키는 행으로의
  link로 쓰이며, 행은 자기 key를 통해 link를 가진다.
- **block 배치.** block table의 배치 외래 key — 그 `store block <fk>` setting이 이름 붙이는 외래 key — 의
  column은 non-null이다. nullable이면 block table 이름에 위치한 schema 오류다. 근거: block 행은 그 배치
  외래 key가 참조하는 행의 scope 안에 서며([document model](20-document-model.md#storage)) NULL은 어떤 행도
  참조하지 않으므로, 그 자리에 NULL을 가진 행은 쓰일 자리가 없다. SQL 행은 자리가 필요 없으므로 dbspec은 nullable
  외래 key column을 받아들인다. 이 catalog 규칙은 여기서 정한다. 이 규칙은 모든 block
  table에 성립한다.

## Key

corpus는 `aes`, `aes_version`, `blind_index`를 선언하고 orm이 저장하는 것을 저장한다: 암호문, 행의 key version, blind index.

- **client는 자기 key를 쥔다.** orm 요청에서는 SQL과 같이 client가 암호화하고 hash한다: 연결 설정이 key를 준다(orm
  `COD-8`). executor는 저장값만 받으므로 key가 필요 없다.
- **엔진은 주입으로 key를 받는다.** 환경 변수 `FLOWMARK_KEYRING`이 keyring 파일을 가리킨다. 파일은 orm 설정과 같은 모양의 JSON
  객체 `{"current": <version>, "versions": {"<version>": "<key>"}, "blind_index": "<key>"}`이고 `blind_index`는 선택이다.
  모든 명령이 catalog를 읽을 때 이 파일을 읽는다. 이 객체로 읽히지 않는 파일, 선언되지 않은 current version, 빈 key는 그
  변수를 지목하며 catalog load를 실패시킨다. corpus 안의 keyring(corpus가 공개한다)과 symbolic link도 마찬가지다.
- **엔진이 시작하는 쓰기**(CLI, MCP, sidecar)는 aes column의 평문을 받아 orm 규칙을 따른다: 현재 key로 암호화하고, 그 version을
  기록하고, blind index를 쓴다. version column 대입은 `IR_INVALID`이고, 암호화 데이터의 update는 모든 aes column을 대입한다.
  한 version이 행 전체를 설명하기 때문이다. keyring이 없으면 그런 쓰기는 평문을 저장하게 되므로 `CONFIG`다. `rows.load`
  ([operations](40-ops.md#the-faces))는 저장값을 받으므로 그런 쓰기가 아니다.
- **validator와 commit**은 aes column을 행이 기록한 version의 key로 decode한다([constraints](50-constraints.md#codec-values)).
  keyring이 없는 process에서는 그 column을 판정하지 않고 corpus의 나머지를 판정한다.
- **export**([sync](70-sync.md))는 저장된 암호문, version, blind index를 그대로 두므로 key가 필요 없다.

근거: 암호문은 executor의 모든 읽기와 쓰기에 불투명하므로 corpus는 SQL처럼 이 설정을 받는다. corpus에 든 key는 corpus와 함께
공개되므로 밖에서 주입한다. 엔진이 쓴 값은 같은 key를 가진 client가 읽을 수 있어야 하므로 엔진은 orm 규칙을 지키고 stage를
재구현하지 않는다.

## 이름 규칙

- **예약된 table 이름.** 대소문자를 무시하고 `CON`, `PRN`, `AUX`, `NUL`, `COM0`–`COM9`, `LPT0`–`LPT9`와
  같은 table 이름은 schema 오류다: Windows의 예약된 file 이름이기 때문이다.
- **부모의 markdown column과 같은 이름을 가진 자식 table**은 schema 오류다.
- **heading 깊이.** layout이 H6를 넘는 heading을 필요로 하면 schema 오류이며, catalog를 읽을 때 정해진다. table의 행은 body가 아닌 markdown column이나 child block을 가질 수 있으면 자기 row scope 수준에 section을 쓰고, body를 가질 수 있으면 그 한 단계 위에 detail heading을 쓴다. 둘 중 어느 것도 가질 수 없는 table은 block heading 아래에 heading을 쓰지 않으므로, row scope가 H6를 넘어도 된다. 근거: layout이 쓰지 않는 heading은 layout이 필요로 하는 heading이 아니다. `markdown` column도 child block도 없는 block table은 detail section이 없으므로 block row 아래 level 7에 설 수 있다.

## Install

`flowmark install <corpus>`는 catalog에서 저장 뼈대 — 각 `files` table의 폴더와 각 `document` table의
file `<table>.flowmark.md`이며, 빈 블록은 쓰이지 않으므로 블록 섹션은 없다 — 를 `CREATE TABLE`이 저장소를
만들듯 만든다. 또한 `<corpus>/.gitattributes`에 `*.flowmark.md text eol=lf` 줄을 더하며, 파일이 없으면 만들고 다른 줄은 유지한다. 그래서 git checkout은 모든 플랫폼에서 LF를 유지한다([canonical format](10-canonical-format.ko.md#encoding과-줄-구조)). `markdown://`에 대한 orm의 schema install은 `CAPABILITY_UNSUPPORTED`다.

## Introspection

introspection은 모든 문서를 catalog에 대해 검증하고 catalog를 돌려준다. 실패한 문서는 위치가 있는
진단이다. catalog 없는 corpus에는 돌려줄 schema가 없으며, catalog 초안을 만드는 도구는 이 spec에 속하지
않는다.

- 돌려주는 catalog는 `dbspec/dbspec-set.json`의 문서 set이다: 각 문서를 dbspec의 canonical form(orm `docs/dbspec.md`,
  Canonical form)으로, 그 corpus path와 문서 이름과 함께 file 이름 순서로 돌려주고, set의 manifest hash를
  함께 돌려준다. 근거: catalog는 schema의 유일한 출발점이므로 introspection은 catalog를 돌려주며 행에서
  유도한 것은 돌려주지 않는다. orm의 database introspection은 dbspec 문서를 돌려주고, canonical form은
  dbspec 자신의 문서 형식이다 — 모든 선언, markdown setting, comment를 유지하므로 돌려준 set을 다시 읽으면
  같은 manifest hash가 된다. manifest hash는 client의 생성 코드를 검사하는 값이므로(`SCHEMA_HASH_MISMATCH`)
  도구는 그것으로 자신의 코드가 corpus와 맞는지 안다. catalog는 orm의 `engine/dbspec`으로 읽고 그
  `Emit`으로 쓰며, 두 번째 parser나 emitter를 두지 않는다.
- 진단은 validator의 모든 진단([constraints](50-constraints.ko.md#진단))이며 file, line, column,
  rule, message 순으로 정렬한다. catalog가 load되면 문서의 실패 여부와 관계없이 catalog를 진단과 함께
  돌려준다. 근거: 행은 schema를 바꾸지 않으므로 실패한 문서는 catalog에 대해 아무것도 말하지 않는다.
  database의 introspection도 행이 무엇을 담든 schema를 돌려준다. catalog를 돌려주지 않으면 문서를 고치는
  사람에게서 schema를 숨기게 된다. 진단이 하나도 없을 때에만 corpus가 유효하다.
- load되지 않는 catalog는 catalog 없이 그 `catalog` 진단만 돌려준다: validator는 catalog를 통해 문서를
  읽으므로 catalog 없이는 실행할 수 없다. `dbspec/`이 없는 corpus는 `dbspec`에 위치한 `catalog` 진단
  `no dbspec/ directory`이고, set 파일이 없는 corpus는 `dbspec/dbspec-set.json`에 위치한 `catalog` 진단
  `no dbspec/dbspec-set.json`이다.
- 명령줄은 `flowmark introspect <corpus>`이며 `<corpus>`는 corpus root다. 표준 출력에 JSON 객체 하나
  `{"catalog": {"manifest_hash", "documents": [{"file", "name", "text"}]}, "diagnostics": [{"file",
  "line", "column", "rule", "message"}]}`를 쓰며, catalog가 load되지 않으면 `"catalog": null`, 진단이
  없으면 `"diagnostics": []`다. 진단이 없으면 0으로 끝난다. 진단이 있으면 객체를 똑같이 쓰고 1로 끝나며,
  명령이 실패하면 실패를 표준 오류에 쓰고 표준 출력에는 아무것도 쓰지 않고 1로 끝난다. 잘못된 명령줄은
  [canon](40-ops.ko.md#canon-연산)처럼 2다. 근거: 문서의 text는 여러 줄에 걸치고 그 답은 도구가
  읽으므로, IR-executor protocol의 message처럼 하나의 JSON 값이다. introspection은 검증하므로 진단은
  검증 실패처럼 명령을 실패시킨다.
