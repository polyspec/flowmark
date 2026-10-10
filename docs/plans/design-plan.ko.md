<!-- doc-id: plans-design-plan -->
<!-- source-sha256: 3edf1a6a37b1a87ea8486abe3d802e5747f11c62f5ed54abaf1ba11a83b5c7b5 -->
# flowmark 설계 계획

Revision 2 — 3개 lane의 적대 감사 결과(engine core, orm 통합, 운영)와 multi-platform 지시를 아래 판정에 접는다. 감사의 blocker는 전부 판정이 되었고, 부수지 못한 주장은 문서 끝에 남긴다.

Revision 3 — [markdown database 계획](markdown-database-plan.ko.md)이 철회한 판정을, 그것을 철회하게 한 사실과 함께 정정한다:

- **Seam.** 이 계획은 markdown executor를 orm 안에 두었다. orm은 query layer다: client는 자기 process 안에서 plan하고 모든 database에 driver로 접속한다(orm `docs/protocol.md`, `docs/config.md`). 그래서 orm 안의 executor는 client마다 하나씩 존재하게 된다 — 아래의 "Go-only" 단서가 그 증상이었다. code도 그 판정을 따른 적이 없다: orm은 dbspec setting 두 개를 받았고 flowmark는 orm에서 아무것도 import하지 않았다. flowmark가 엔진이고 orm은 DSN으로 IR executor로서 접속한다.
- **Schema stamp와 단독 fragment.** 모든 문서의 stamp는 schema를 행마다 복사한다. corpus root의 `dbspec/` catalog가 유일한 schema다.
- **task-ID 문법.** key는 불투명하다. 계층은 `parent` column이고 대체는 link이므로 ID 문법, 그 정렬, renumber codemod는 사라진다.
- **`identity` 거절, advisory `audit`, column limit으로서의 WIP gate, operation 단위만의 lock, corpus revision**은 계획이 적은 대로 대체한다.
- **T8.5** — 문서 안 Mermaid 블록을 전이 authority로 쓰는 것 — 은 이 계획의 판정이 아니었으며 함께 철회한다.
- **읽을 수 있는 문서.** corpus 문서는 일반 markdown 문서처럼 읽히는 `*.flowmark.md` file이다: doc-id header, 행 file의 frontmatter, provenance header, twin, `dbspec/flowmark.json`은 철회된다. `files` 행의 key는 file 이름이고 foreign key는 link다([markdown database 계획](markdown-database-plan.ko.md), "읽을 수 있는 문서").

아래 절은 정정된 판정을 담고, spec이 그 전부를 담는다.

## 배경

사용자는 markdown으로 engineering spec을 관리하며 checklist(상태 5종 `[ ]` `[~]` `[o]` `[!]` `[-]`, dotted 계층 ID, 선행/후속 의존), changelog, commit 규칙으로 개발을 운영한다. 문제: 문서 기반 운영에는 **멱등하고 DB 수준의 강제력이 없다** — 형식 위반, dangling foreign key, 불법 status 전환이 아무도 막지 못한다.

**flowmark의 정체성 (사용자 확정)**: **markdown 문법을 가진 텍스트 DB**. 파일 자체가 database이며 외부 DB 없이 standalone으로 완전 동작한다. **flowmark가 엔진**이고(Revision 3 — orm은 IR executor로서 접속하는 query layer) **통합·응용 layer**다 — database, GitHub, Jira 등 tracker와의 양방향 동기화, 그리고 그 위에서 local 칸반 보드와 issue tracker(GUI는 또 하나의 thin consumer): GitHub과 Jira에도 보드는 있지만 여기서는 data를 로컬에서, git 안에서, 오프라인으로 관리한다. 핵심 가치: 저장 형식이 사람이 읽는 문서이고, git이 audit·branching·rollback을 제공한다.

**사용자 결정사항**:

1. polyspec/orm은 markdown에 mysql/pgsql/sqlite 옆의 **4번째 database**로 접속한다 — flowmark가 서비스하는 IR executor를 고르는 DSN `markdown://`을 통해서(Revision 3). face는 자체 write logic이 없는 thin adapter다.
2. Schema 방향: **dbspec이 root grammar이자 유일한 authority** — corpus root의 `dbspec/` catalog가 모든 table과 그 저장을 선언하며 문서는 schema 사본을 갖지 않는다 (상세 §4, Revision 3).
3. 문서는 **EN/KO 쌍을 day 1부터** (orm house style, kit 호환).
4. **polyspec/kit을 지금 vendor** — 5번째 상태 `[-]`는 **polyspec/kit 저장소에 upstream** 한다 (orm의 `scripts/kit`은 hash-lock된 vendor copy라 orm에서 직접 고쳐선 안 되고 `make kit-sync KIT_TAG=<tag>`로만 반영된다). 사용자가 kit에 대한 권한을 위임했다.
5. 이번 session의 deliverable = **설계 문서 + repository bootstrap** (구현은 checklist의 후속 phase로 추적). 감사의 범위 지적에 따라 spec 문서 자체는 다음 session(T1b)으로 분리하고, 이번 session은 T1a를 닫는다.
6. Sync projection 파일: 실제 markdown 파일이 disk에 존재. `readonly`(기본) / `writable` 옵션 (상세 §7).
7. Repository는 **github.com/polyspec/flowmark** — polyspec organization 안의 독립 repository.
8. **Multi-platform은 hard requirement. 편법 금지.** 현재 요구사항을 완전히 충족하는 가장 단순한 구현을 선택한다 (kit 자신의 규칙을 여기에 적용): flock 없음, POSIX 전용 durability 가정 없음 — lock은 kit의 `holder-lock` 선례(lock file + holder 신원 + stale 판정, 순수 runtime primitive)를 따르고, crash 안전성은 OS별 directory fsync가 아니라 journal replay로 얻는다.
9. **GUI는 자기 repo의 soksak plugin으로 출시되고, flowmark는 soksak의 sidecar로 서비스한다.** 하나의 Go binary가 세 개의 thin face을 갖는다 — CLI, MCP server, soksak sidecar(soksak core 명세의 protocol v1: 표준 입출력 위의 JSON lines, surface·root routing, host 소유 lifecycle). 저장소는 단일 언어 Go에 `package.json`과 `sidecar.json` manifest를 얹은 형태이고, 언어별 package는 요구사항이 이름을 붙일 때 만든다.

## 검증된 근거 (polyspec checkout에서 직접 확인)

- `orm/docs/protocol.md:12,96` — IR kind는 bounded: `one|all|count|group_count|sum|avg|paginate|insert|update|delete|restore`. request는 SQL text를 담지 않는다 → **IR request를 소비하는 native markdown executor는 bounded 문제**. 감사가 표현력을 확인했다: 전 kind와 Order/Pred/Func 표면이 in-memory corpus 위에서 계산 가능하며, 열린 문제는 명세였지 표현력이 아니었다.
- `orm/docs/dialects.md:184` — 3개 database가 collation을 binary codepoint order로 통일한다. 이 규칙은 **text 비교**의 규칙이며 엔진이 따른다 (§2의 ordering 판정 참조).
- `kit/scripts/kit/documents-stamp.mjs`, `check-documents.mjs` — `source-sha256`는 **EN→KO 번역 sync marker**(Korean twin이 English 파일 raw bytes의 sha256를 보유). 데이터 drift 감지는 `.flowmark/` sidecar state로 설계하며, flowmark는 같은 marker에 **raw 파일 bytes hashing**을 쓴다 (kit 호환; 정규화는 hash가 아니라 검사에서 한다).
- `kit`은 독립 저장소(`polyspec/kit`)이며, consumer는 `make kit-sync KIT_TAG=<tag>`가 쓰고 `make kit-check`가 검증하는 `scripts/kit` + `tests/kit`의 hash-lock vendor copy를 가진다. vendored file은 kit에서만 바꾼다.
- `kit/scripts/kit/checklist.mjs` — 상태는 tracker별 data다. `[-]` 불가 상태는 이제 두 수준에서 읽힌다 (kit K13으로 upstream, branch `feat/fifth-state-K13` merge 대기 중).
- kit 문서 규칙: 모든 English 문서에 Korean twin 필요(doc-id 일치, explicit anchor 일치, fenced block 일치, twin이 source-sha256 보유), plans/와 CHANGELOG도 twin 검사 대상.

## 핵심 설계 (spec 문서가 확정할 내용)

### 1. Architecture와 seam

```
application code ── orm client (Go/PHP/Python/Rust/TS) ── IR executor ── DSN markdown:// ──┐
                                                                                          ▼
flowmark = the markdown database engine (one Go binary)
   ├─ codec: strict parser + canonical renderer of the storages (two-clause idempotency, §2)
   ├─ validator: catalog constraints (type/PK/UNIQUE/FK/CHECK/state machine and history)
   ├─ IR executor over in-memory indexes derived from the parsed corpus
   ├─ write path: lock → conflict check → validate → stage → prepare record → renames → commit
   ├─ .flowmark/: lock, tx journal, server token, parsed-row cache
   ├─ faces: server · CLI · MCP · soksak sidecar
   └─ sync engine: md→db export / db→md materialize (§7)
```

**Seam 판정 (Revision 3)**: orm은 query layer다. client는 자기 process 안에서 plan하고 DSN scheme이 고른 driver로 각 database에 접속한다. scheme `markdown://`은 **IR executor**를 고른다: client는 IR 요청을 검증하고 plan하지 않으며 flowmark server로 보내고, server는 행이나 orm 오류 하나로 답한다 — `getQuery()`, statement event, `subscribe()`, `force_index`는 `CAPABILITY_UNSUPPORTED`다(지원 불가 능력에 대한 orm의 선례). 엔진은 flowmark에 한 번 존재하고, 각 orm client는 얇은 executor client를 가지며 Go client가 먼저다. flowmark는 orm의 `engine/dbspec`와 `engine/ir` package를 library로 쓴다. Revision 2는 executor를 orm에 두었고, 그러면 다섯 client 각각에 엔진이 들어갔을 것이다.

- SQLite mirror는 부정한다: mirror↔corpus 동등성이 영구적인 bug class가 된다. mirror를 인정하더라도 conformance 조건은 (a) full-rebuild equivalence (b) write path 부재다.
- IR boundedness 주장은 request layer만 대상으로 한다. **API 동등성은 주장하지 않는다** — capability table(§6)이 계약이며, orm의 `CAPABILITY_UNSUPPORTED`/`OPERATOR_NOT_ALLOWED` 전통을 따른다.

### 2. Canonical format (`10-canonical-format.md`)

- **멱등성은 dbspec의 2절 명제다** — 단일 byte-identity 주장이 아니라: canonical 입력 `s`에 대해 `emit(parse(s)) == s`, 임의의 valid 입력 `d`에 대해 `emit(parse(emit(d))) == emit(d)`. renderer는 **구조 skeleton** — `files` 행의 field list, heading line, 행들의 item/table line — 만 소유하고(Revision 3), 보존 구간의 payload bytes(prose, fenced 내용)는 결코 다시 쓰지 않는다. 비-canonical 입력은 `format.noncanonical` diagnostic과 별도의 일회성 `flowmark canon` migration transaction으로 처리하며, 논리적 write의 부수 효과로 정규화하지 않는다.
- body와 모든 `markdown` column 값은 fenced block 밖의 heading level로 경계가 정해지는 행 scope의 섹션이다. 없는 섹션은 NULL, 비어 있는 섹션은 빈 text다 (Revision 3).
- **Ordering**: IR `order`는 column의 type으로, text는 codepoint로 정렬한다. 없는 읽기는 저장 순서 — 형제를 order column이나 key 순서로 둔 tree의 전위 순회 — 를 돌려준다. `group_count`는 NULL을 같은 group으로 처리한다 (SQL 동등성). key는 불투명하다: task-ID 문법, 그 segment별 정렬, suffix 규칙, `flowmark next-id`는 철회한다(Revision 3). `identity`와 `key_prefix`가 key를 할당한다.
- **List 행**은 상태 glyph, key, 제목, 오른쪽부터 parse하는 끝의 `column:value` field를 갖는다. `markdown` column은 field나 cell이 아니며, 한 줄 자리에 줄바꿈을 가진 값은 거절한다.
- **읽을 수 있는 값** (Revision 3): corpus 문서에는 frontmatter도 comment도 없다. 일반 markdown 문서처럼 읽힌다. `files` 행의 scalar는 H1 뒤의 field list이며, dbspec literal이 아니라 읽을 수 있는 형식(날짜, uuid, `bytes`는 padding 있는 base64)으로 쓴다. 모든 관계는 link다.
- corpus 문서에는 twin이 없다 (Revision 3): 행 file은 doc-id도 `source-sha256`도 갖지 않는다. EN/KO 쌍과 `source-sha256`(English 파일 raw bytes의 sha256, kit 호환)은 이 repository 자신의 문서에 속한다.

### 3. Document-table model (`20-document-model.md`)

- 각 table은 저장 하나를 선언한다(Revision 3): `store files`(table 폴더에 행마다 file 하나), `store document list|table`(모든 행이 `<table>.flowmark.md`에), `store block <fk> list|table`(부모 행 안의 행). aggregate냐 split이냐는 저장 선언이며 논리 관계는 같다.
- `files` 행은 file `<table>/<key>.flowmark.md`이며, H1이 제목이고 scalar column이 그 뒤의 field list다 (Revision 3). key는 file 이름이고, 계층은 `parent` column이며 list에서는 들여쓰기로 쓴다.

### 4. Schema (`30-schema.md`) — catalog가 schema다

- **dbspec이 root grammar이자 유일한 authority.** corpus root의 `dbspec/*.dbs` set이 catalog다(Revision 3). 문서는 schema 사본도 stamp도 갖지 않고, 내장 fragment를 가진 단독 문서는 철회한다: schema는 law이고 data는 row이며, 문서마다 law의 사본을 두면 사본의 이탈이 그 자체로 bug class가 된다. orm의 engine, IR, conformance는 dbspec manifest hash에 묶여 있고, dbspec의 parser와 type system은 library로 재사용한다.
- **Markdown 고유 schema 표면은 manifest-level setting이지 render되는 type이 아니다** (introspection 정합성, `updated`/`soft_delete` 선례): `store`, `key_prefix`, `title`, `body`, `order`, `checkbox`, `markdown`, 그리고 state machine의 `initial`, `history`, `limit`은 SQL backend가 무시하는 setting이며, 실제 database는 이들을 평범한 column으로 introspect한다.
- **`identity`는 허용한다** — 모든 markdown table에서 lock 아래 `max+1`로 할당한다(Revision 3). git 되감기에서의 key 재사용은 받아들인다 — 할당 기록은 sequence가 아니라 git history다.

### 5. Ops와 transaction (`40-ops.md`)

- **Commit protocol** (순서가 규범적): post-image 파일을 `.flowmark/tx/<id>/`에 content-addressed로 쓰고 fsync; 모든 대상의 pre-image를 stage로 복사하고 fsync; 모든 rename과 pre/post hash를 적은 **prepare record**를 쓰고 fsync; runtime의 atomic rename으로 rename; commit record를 쓰고 fsync; stage 정리. **복구는 corpus를 열 때마다 실행한다**: prepare record가 있으면 완료, 없으면 stage 폐기, 대상의 현재 hash가 기록된 어느 쪽과도 다르면 거부하고 보고한다(crash 사이에 사람 편집이 들어왔다는 뜻 — 결코 조용히 덮어쓰지 않는다). 정전 이후의 durability는 OS별 directory fsync가 아니라 journal replay에 의존한다.
- **하나의 portable lock** (multi-platform 판정): 모든 작업 — 읽기와 쓰기, 어느 face에서든 — 은 holder 신원·stale 판정을 갖는 같은 lock file을 획득한다(kit `holder-lock` 선례) — timeout은 holder를 보고한다. lock은 작업 하나 동안, 또는 server transaction 하나의 `begin`부터 `commit`이나 `rollback`까지 잡는다(Revision 3). 닫힌 연결이나 유휴 transaction은 rollback하고 lock을 푼다. commit 시 충돌(읽은 뒤 대상이 바뀜)은 file과 line과 함께 실패한다 — face는 다시 읽고 한 번 재시도하며 orm client는 `WRITE_CONFLICT`를 받는다 — silent merge 없음.
- **Server**(Revision 3)는 loopback에 bind하고 유지되는 token을 갖는 상주 face다. DSN은 `markdown://127.0.0.1:<port>/<corpus>?token=<token>`이다.
- **검증은 commit 시점에 충돌 검사 뒤 lock 아래에서 corpus를 새로 읽어 수행한다** — BEGIN snapshot도 예측된 touch file 집합도 아니다.
- corpus revision과 `CORPUS_CHANGED`는 철회한다(Revision 3): `paginate`는 요청 하나이고, 호출을 넘어 읽는 face는 없다.
- **비용 모델**: 40-ops가 per-command p95 latency 목표를 선언한다(corpus envelope 기준 수백 ms 단위); file content hash를 key로 하는 `.flowmark/cache/`의 parsed row cache가 변하지 않은 file의 재parse를 피한다; commit은 transaction이 바꿀 수 있는 모든 제약을 lock 아래 다시 읽은 corpus에서 검사하며, 전체 `flowmark validate`는 on demand와 catalog 변경 시에 실행한다.
- Walk 규칙: `dbspec/`, `.flowmark/`, 숨김 항목 밖의 `*.flowmark.md` 파일만 읽는다; symlink된 문서나 `files` 폴더는 `corpus.walk`이고, 그 밖의 symlink는 database 밖이다; hard link는 규칙이 아니다(document model).

### 6. Constraints와 capability table (`50-constraints.md`, `60-dialect.md`)

- Strict parse, type/PK/UNIQUE/FK(exact byte match; 대소문자 무시 시 key 유일), CHECK. soft-delete된 row는 unique key를 계속 차지하고 FK를 계속 만족한다(SQL 동등성 — soft delete가 가짜 판정을 일으키지 않게 명시). `aes`, `aes_version`, `blind_index`는 corpus에서 schema 오류다: key는 orm client를 떠나지 않는다(Revision 3).
- **state machine은 dbspec-level manifest setting**(전이, terminal, 필수 column, 그리고 — Revision 3 — `initial`, `history`, `limit`)이다. 모든 엔진 전이는 이력 행을 쓰고, 엔진 밖에서 바뀐 상태는 이력 빈틈이나 끊긴 사슬로 보고하며 repair command가 있다. 응용 schema는 완료된 항목을 `supersedes` link로 대체한다. 엔진은 WIP 규칙을 강제하지 않는다 — 상대 gate는 제거하고 사용자의 규칙은 작업 규칙으로 남는다.
- **사람의 write는 DB의 writer 독점 전제를 무효화한다; 각 기능은 그에 대한 판정을 갖는다**: FK action은 **restrict만 허용**(`cascade`/`set_null`은 catalog를 읽을 때 거부 — 엔진을 우회하는 writer에게 구현 불가능하므로); `audit`와 `immutable`은 corpus에서 schema 오류이며 상태 이력이 대신한다(Revision 3); 모든 non-null column은 쓰며 엔진이 insert에서 기본값을 쓴다.
- **Capability table**(`60-dialect.md`)은 IR executor wire 아래의 통합 계약이다: `update|share` lock은 문서화된 no-op, `*_nowait`는 거부; `isolation`/`timeoutMs`는 `CAPABILITY_UNSUPPORTED`, `fulltext`/`Lb`는 `OPERATOR_NOT_ALLOWED`; `on_duplicate` 충돌 대상은 그 갈라짐과 함께 적는다; 바뀐 대상은 `WRITE_CONFLICT`로 실패; `getQuery`, statement event, `subscribe`, `force_index`는 `CAPABILITY_UNSUPPORTED`; savepoint는 중첩 stage.
- markdown plan lifecycle(orm의 dbplan/apply/recover 장치)은 **v1 범위 밖**: v1은 corpus 생성과 additive column 추가를 지원하고, markdown corpus의 dbplan chain은 거부한다(80-migration이 범위와 이유를 서술).

### 7. Sync와 adapter (옵션, `70-sync.md`)

- sync engine은 **하나의 계약을 가진 adapter architecture**다; database가 첫 adapter이고 GitHub(issues/projects)과 Jira가 따른다: table별 truth ownership, 소유 system을 밝히는 문서 밖에 둔 sync provenance(그 위치와 형식은 open question 5(70-sync)가 정한다), git을 인식하는 drift 처리, merge base로 기록되는 materialized-from revision, 명시적 일회성 command(live daemon 없음). 외부 tracker의 항목은 corpus row로 materialize된다 — checklist app의 item model이 곧 local issue model이므로 local board와 remote tracker가 같은 row의 두 projection이다.
- md→db: derived export (재구축 가능). db→md(그리고 모든 adapter→md): materialize. validator는 adapter-owned 문서의 편집을 거부한다(readonly 기본); `writable`은 검증 후 re-ingest한다.
- **readonly drift는 git을 인식한다**: working tree가 HEAD와 다르면 HEAD로 복구하고 보고; tree가 깨끗한데 HEAD가 마지막 ingested version과 다르면 수용하고 re-baseline한다. adapter-owned path의 merge conflict는 양쪽을 버리고 re-materialize로 해결한다 — 그곳의 손 merge는 diagnostic이다.
- export는 catalog에서 orm의 SQL dialect renderer로 렌더한다(Revision 3). changelog projection은 이력 행 — 파일 안의 data — 에서 유도하며 gitignore된 journal에서 절대 유도하지 않는다(fresh clone이 모든 projection을 재구축할 수 있어야 한다).

### 8. Migration (`80-migration.md`)

- v1 범위: catalog에서 `flowmark install`로 corpus 생성, additive column 추가. 저장 이동은 transaction 하나다. renumber codemod는 ID 문법과 함께 철회한다(Revision 3). additive-first; rename은 명시적 step; dbplan chain은 `CAPABILITY_UNSUPPORTED`(§6 참조).

### 9. 응용 schema (`100-checklist.md`)

- `flowmark.dbs`(Revision 3): `items`(parent, 이력을 갖는 5상태 기계, 제목, body, 담당자, date/datetime 쌍인 시작과 마감, priority, position), `people`, `item_links`(blocks, relates, duplicates, supersedes), `item_history`, `views`.
- **View는 데이터이자 thin consumer다**: 칸반, 목록, 간트 view는 `views`의 행이다. 보드는 엔진으로 읽고 같은 operation으로 쓴다 — 불법 drag는 불법 CLI 쓰기와 똑같이 실패한다. view 전용 write path는 없고, WIP limit을 지어내지 않는다.
- 이 repository는 corpus가 아니다(Revision 3): 그 checklist는 손으로 관리하는 kit 문서로 남으므로 cutover는 철회한다. `make validate`와 prose ID checker는 옛 엔진과 함께 제거되며, `make refs-check`는 계속 kit이 추적하는 checklist를 검사한다.

## Session 계획 (이번 session은 T1a, 다음 session은 T1b)

**T1a — 감사 반영 설계 + bootstrap** (이번 session)

1. flowmark `git init`, `.gitignore`(`.flowmark/`), `README.md`+`README.ko.md`, `AGENTS.md`+`AGENTS.ko.md`(운영 규칙: checklist discipline, conventional commits, [o]↔changelog↔commit 단위, twin workflow — 한국어에서 발견한 수정은 다음 stamp 전에 English 파일로 이식, 문서 commit에 대한 interactive history 편집 금지), `CHANGELOG.md`+`CHANGELOG.ko.md`(`## Unreleased`). commit: `chore: Bootstrap flowmark repository`.
2. **kit `[-]` upstream** — branch `feat/fifth-state-K13`에서 완료(worktree `kit-fifth-state-K13`, red vector → FIVE_STATES + 원인 규칙 → green, K13 row + AGENTS 쌍 + stamp된 KO checklist). kit의 병렬 작업이 안정화되면 `main`에 merge하고 worktree와 branch를 즉시 제거한다. orm은 사용자가 kit release에 tag를 붙일 때 `make kit-sync`로 채택한다.
3. flowmark에 **kit 디렉터리 전체 vendor**(`scripts/kit` + `tests/kit` + `kit.json` + `.kit/kit.lock.json`, kit-sync 계약 준수, lock은 kit commit을 기록)하고 `config/checklist.json`(list format, `T<n>(.<n>)*(-<n>)?` ID pattern, 상태 5종, active `[~]`)과 `config/documents.json`을 작성한다. commit: `chore: Vendor polyspec/kit tooling`.
4. `docs/plans/execution-checklist.md`+`.ko.md` — 사용자 형식(doc-id, 선행/후속, 원인/완료 기준/Evidence)으로 T1a/T1b/T2–T9 항목을 담고 전 과정에서 갱신한다.
5. 이 설계 계획(revision 2)과 Korean twin — 감사를 접은 상태로 commit.

**T1b — spec 문서** (다음 session; EN/KO 쌍, 문서당 commit 1개, twin stamp): `00-overview`, `10-canonical-format`(§2), `20-document-model`(§3), `30-schema`(§4), `40-ops`(§5), `50-constraints`(§6), 그리고 skeleton `60-dialect`(capability table을 전체 내용으로), `70-sync`, `80-migration`, `90-conformance`, 전체 내용 `100-checklist`, `110-open-questions`. 문서별 완료 기준: 모든 판정이 근거를 동반한 명제로 나타난다 (prose 분량이 아니라 결정 수).

**T9 conformance 계약 (지금 확정)**: markdown vector는 corpus-before + IR request + 기대 corpus-after / 기대 rows이며 Go executor에 대해 실행한다. SQL/binds 비교는 적용되지 않는다(SQL이 없으므로). 다른 client가 executor를 port할 때 교차 비교가 시작된다. (Revision 3: vector는 flowmark 엔진에 대해 실행하고, orm의 executor conformance는 flowmark build를 고정한다.)

**이후 phase (checklist `[ ]` 항목으로만 등록)**: T2 codec(Go), T3 validator, T4 executor, T5 orm markdown-executor 통합(manifest setting, state-machine sub-grammar — orm 저장소 변경), T6 sync(adapter 계약과 database adapter; GitHub·Jira adapter는 그 아래에 등록), T7 세 개의 face — CLI·MCP server·soksak sidecar(protocol v1: stdio JSON lines, surface routing, host 소유 lifecycle; MCP tool 목록 mini-spec 먼저, lock은 작업마다 또는 server transaction마다 잡힌다(§5)), T8 checklist app(local issue model), T9 conformance vectors, T10 자기 repo의 soksak plugin — local 칸반 보드와 issue tracker, 또 하나의 thin consumer로서 view 전용 write path 없음.

## 감사가 부수지 못한 것

- 정합하는 두 flowmark writer 사이에서는 작업 단위 lock + stage/prepare/commit가 갱신을 잃지 않는다 (진짜 구멍은 reader, 사람, crash 창이었다 — 위에서 판정됨).
- dbspec 우선 파생 방향: 반대 방향은 이 계획이 거부하는 DDL-reconciliation conflict class를 실제로 만든다. (Revision 3은 방향을 유지하고 단독 fragment를 철회한다: catalog가 유일한 schema다.)
- corpus 위의 IR 표현력: 전 kind, relation(limit_per_parent, if_parent, flatten, key_by, 복합 key), tuple_in subquery, group_count, restore와 optimistic이 type-directed ordering과 PK순 결정성 아래 모호함 없는 in-memory 정의로 mapping된다.

## Open questions

open question은 `110-open-questions.md`에 있고, 결정되면 spec으로 승격한다.

## Verification (T1a)

- kit branch `feat/fifth-state-K13`: `node --test tests/kit/checklist.test.mjs` 13/13, `make documents-check` green (worktree에서 검증됨).
- flowmark `make kit-check`(lock 온전), `make documents-check`(쌍, doc-id, anchor, fence, source-sha256), 상태 5종 checklist 읽기 — green.
- `git log`(모든 저장소): conventional commit 형식, 기능 단위, 모든 `[o]` 항목에 changelog entry(flowmark) 또는 소유 row(kit) 동반.
- spec 완료 기준(구현 없이 검증 가능): 교차 참조 해석 가능, 개념당 이름 하나, 모든 판정이 근거를 동반한 명제, 2절 멱등성이 그렇게 명시됨.
- T1b가 `10-canonical-format`을 `[~]`로 시작할 수 있는 상태.
