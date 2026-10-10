<!-- doc-id: spec-sync -->
<!-- source-sha256: c4a22cd93f1481a6d6ab0855021b8b82a52b825252bcc2b1775f03768cc296b0 -->
# Sync (범위를 정한 뼈대)

[English](70-sync.md)

이 문서는 sync 엔진의 범위와 확정된 판정을 적는다. 세부 spec은 database export 너머의 adapter가 시작될
때 쓴다.

## 범위

- sync 엔진은 **하나의 계약을 가진 adapter 구조**다. database가 첫 adapter이고 GitHub(issue, project)와
  Jira가 같은 계약 아래 뒤따른다.
- md→db는 파생 export다: database는 corpus에서 다시 만들 수 있다. export는 엔진으로 행을 읽고 catalog의
  runtime model에서 orm의 SQL dialect renderer로 statement를 렌더한다 — SQL을 손으로 렌더하지 않고 자기
  table 이름을 두지 않는다.
- db→md(그리고 모든 adapter→md)는 materialization이다: adapter의 행을 엔진으로 선언된 저장에 쓰고
  provenance header로 표시하며 commit이 이를 기록한다.

## 확정된 판정

- **truth ownership은 table 단위다.** 각 table은 어느 쪽이 authority를 갖는지 선언하고, 다른 쪽은
  projection이다.
- **provenance header**는 adapter가 소유한 각 file의 `<!-- owner: <adapter>:<table>@<sha256-of-own-bytes> -->`다.
  hash는 file 자신의 byte를 덮으므로 검사는 스스로 검증되며 clone에도 안전하다. 범위는 사고 방지이지
  적대자 저항이 아니다.
- **기본은 readonly다**: validator는 adapter가 소유한 file의 편집을 거절한다. `writable`은 검증 뒤 편집을
  받아들이고 다시 ingest한다.
- **drift 처리는 git을 안다**: HEAD와 다른 working tree는 HEAD로 되돌리고 보고한다. HEAD가 마지막으로
  ingest한 version과 다른 깨끗한 tree는 받아들이고 기준을 새로 잡는다. header는 materialize한 원본
  revision을 기록하여 재ingest에 merge base를 준다.
- **adapter가 소유한 경로의 merge 충돌**은 양쪽을 버리고 다시 materialize해서 해결한다. 그곳의 손 merge는
  진단이다.
- **sync는 일회성 command로 실행한다.** server는 엔진의 face이지 sync daemon이 아니다.
- **projection은 file 위의 데이터에서 다시 만든다**: changelog는 이력 행에서 유도하며, ignore된
  `.flowmark/` journal에서 유도하지 않는다.

## adapter를 위해 열어 둔 것

adapter별 field 대응, 일회성 command의 일정, 충돌 보고 형식, adapter 등록 schema.
