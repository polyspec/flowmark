<!-- doc-id: spec-sync -->
<!-- source-sha256: 531eda46374d63fcdbfe6197eb47f53cea39f185eceeeacc9aab84073e69eeb7 -->
# Sync (범위를 정한 뼈대)

[English](70-sync.md)

이 문서는 sync 엔진의 범위와 확정된 판정을 적는다. 세부 spec은 database export 너머의 adapter가 시작될
때 쓴다.

## 범위

- sync 엔진은 **하나의 계약을 가진 adapter 구조**다. database가 첫 adapter이고 GitHub(issue, project)와
  Jira가 같은 계약 아래 뒤따른다.
- md→db는 파생 export다: database는 corpus에서 다시 만들 수 있다. export는 엔진으로 행을 읽고 catalog의
  runtime model에서 orm의 SQL dialect renderer로 statement를 렌더한다 — SQL을 손으로 렌더하지 않고 자기
  table 이름을 두지 않는다. 사용할 수 없는 table이 필요한 export는 file과 진단을 밝히며 `CONSTRAINT`로 실패하고 아무것도 쓰지 않으며, 다른 table만 export하지 않는다([canonical format](10-canonical-format.ko.md#진단)). `type` 진단이 있는 값을 읽는 export는 같은 방식으로 실패하고 아무것도 쓰지 않는다.

- db→md(그리고 모든 adapter→md)는 materialization이다: adapter의 행을 엔진으로 선언된 저장에 쓰고
  sync provenance로 표시하며 commit이 이를 기록한다. provenance는 document 안에 있지 않으며, 그것이 어디에
  있는지는 open question 5([open questions](110-open-questions.md))다.

## export

- export는 무엇이든 쓰기 전에, read 요청이 읽는 것과 같이 읽은 corpus 하나에서 통째로 계산된다: 실패한
  export는 statement를 내지 않으므로 아무것도 쓰지 않는다.
- 그 statement는 먼저 orm의 renderer(`dbspec.RenderStatements`)가 catalog의 document set에 대해 대상
  dialect — MySQL, PostgreSQL 또는 SQLite — 로 쓰는 schema statement를 renderer의 순서대로 두고, 이어서
  행마다 insert 하나를 둔다. 그 insert는 orm의 planner가 catalog의 runtime model의 `insert` 요청에서
  compile하며, 그 요청은 table의 모든 column을 값이나 NULL로 할당한다. 각 insert는 planner가 쓴 bind slot과,
  read가 답하는 runtime value model JSON으로 된 요청 parameter를 함께 갖는다
  ([capability table](60-dialect.ko.md#wire)). 근거: 각 dialect의 SQL text, quoting, placeholder는
  orm의 것이므로 export는 아무것도 렌더하지 않는다. insert가 뺀 column은 저장된 값 대신 database의 default를
  받는다.
- 모든 table의 모든 행을 export하며, soft delete된 행도 포함한다. 근거: database는 통째로 다시 만들어지고,
  그 database의 read는 corpus의 read처럼 그 행들을 뺀다.
- table은 foreign key의 순서로 채운다: table은 그 foreign key가 참조하는 다른 모든 table 뒤에 오고, 동률은
  schema statement가 table을 만드는 순서를 따른다. table 사이 참조의 cycle에 든 table은 다른 table이 먼저 올
  수 없게 된 뒤 그 생성 순서로 온다. 한 table의 행은 저장 순서([document model](20-document-model.ko.md#순서))로
  서므로 부모 행이 자식보다 앞선다. 근거: statement마다 foreign key를 검사하는 database는 참조하는 행이 아직
  insert되지 않은 행을 거절한다. 그런 cycle에서 서로 참조하는 행은 foreign key 검사를 미루는 database에만
  들어간다. export는 저장된 값과 다른 값을 써서 cycle을 끊지 않는다.
- 같은 corpus는 매번 같은 statement를 낸다.
- identity column이 있는 table은 orm의 `load` insert(identity column의 값을 받는다)로 저장된 key 그대로 행을
  적재하고, 그 행 뒤에 orm의 `restart_identity` statement가 오며, 이 statement는 identity를 table의 가장 큰
  key 너머로 옮긴다. 그래서 다른 행이 참조하는 key가 지켜지고 database가 다음에 생성하는 key는 그 뒤를
  잇는다. 근거: key는 다른 행이 참조하는 행의 identity이며, 새 key를 생성하는 export는 그 참조를 깬다.
- codec column이 있는 table은 export를 `CAPABILITY_UNSUPPORTED`로 실패시키며, 그 table과 column을 밝힌다.
  근거: codec column은 encode된 값을 저장하는데 orm의 insert는 decode된 값을 받아 encode하며, 엔진은 아직
  저장된 값을 decode하지 않는다. orm을 기다린다(open question 6).
- parameter를 database driver에 bind하고 statement를 실행하는 일은 database executor의 일이며, orm은 이를
  client 안에 둔다. orm이 이를 드러내기 전까지 export는 statement에서 끝난다(open question 6).

## 확정된 판정

- **truth ownership은 table 단위다.** 각 table은 어느 쪽이 authority를 갖는지 선언하고, 다른 쪽은
  projection이다.
- **provenance header는 철회된다.** corpus 문서는 comment를 갖지 않으므로
  ([canonical format](10-canonical-format.md)) `<!-- owner: … -->` marker와 문서 자신의 byte에 대한 hash는 존재하지 않는다. adapter가 소유한 문서의
  sync provenance는 document 밖에 있으며, 그것이 어디에 있는지는 open question 5다.
- **기본은 readonly다**: validator는 adapter가 소유한 file의 편집을 거절한다. `writable`은 검증 뒤 편집을
  받아들이고 다시 ingest한다.
- **drift 처리는 git을 안다**: HEAD와 다른 working tree는 HEAD로 되돌리고 보고한다. HEAD가 마지막으로
  ingest한 version과 다른 깨끗한 tree는 받아들이고 기준을 새로 잡는다. materialize한 원본 revision은
  open question 5가 정하는 곳에 기록되며, 재ingest에 merge base를 준다.
- **adapter가 소유한 경로의 merge 충돌**은 양쪽을 버리고 다시 materialize해서 해결한다. 그곳의 손 merge는
  진단이다.
- **sync는 일회성 command로 실행한다.** server는 엔진의 face이지 sync daemon이 아니다.
- **projection은 file 위의 데이터에서 다시 만든다**: changelog는 이력 행에서 유도하며, ignore된
  `.flowmark/` journal에서 유도하지 않는다.

## adapter를 위해 열어 둔 것

adapter별 field 대응, 일회성 command의 일정, 충돌 보고 형식, adapter 등록 schema, 그리고 adapter가 소유한
document의 sync provenance가 놓이는 곳(open question 5).
