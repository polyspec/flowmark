<!-- doc-id: spec-sync -->
<!-- source-sha256: 1c165179d07a1a1085bf198e46aaf91a82699587998507c9caf4c3f76004103d -->
# Sync (범위 skeleton)

[English](70-sync.md)

이 문서는 sync engine의 범위와 확정된 판정을 서술한다. 상세 명세는 T6 시작 시 작성한다.

## 범위

- sync engine은 **하나의 계약을 가진 adapter architecture**다. database가 첫 adapter이고 GitHub
  (issues, projects)와 Jira가 같은 계약 아래에 온다.
- md→db는 derived export다: database는 corpus에서 재구축 가능하다.
- db→md (그리고 모든 adapter→md)는 materialization이다: 실제 markdown 파일이 disk에 존재하고,
  provenance header로 표시되며, commit이 기록한다.

## 확정된 판정

- **Truth ownership은 table 단위다.** 각 table이 어느 쪽이 authority인지 선언하고, 다른 쪽은
  projection이다.
- **provenance header**는 `<!-- owner: <adapter>:<table>@<sha256-of-own-bytes> -->`다. hash는
  materialize된 파일 자체 bytes를 대상으로 하므로 검사가 자기 검증되고 clone 이후에도 유효하다.
  검사 범위는 사고 방지이고 공격자 방지가 아니다.
- **readonly가 기본값**이다: validator는 adapter 소유 문서의 편집을 거부한다. `writable`은 검증
  후 편집을 받아들이고 re-ingest한다.
- **drift 처리는 git을 인식한다**: working tree가 HEAD와 다르면 HEAD로 복구하고 보고한다. tree가
  clean하고 HEAD가 마지막 ingested version과 다르면 수용하고 re-baseline한다. header가
  materialized-from revision을 기록하므로 re-ingest가 merge base를 가진다.
- **adapter 소유 path의 merge conflict**는 양쪽을 모두 제거하고 re-materialize로 해결한다. 그
  위치의 수작업 merge는 diagnostic이다.
- **명시적인 일회성 command만 존재한다** — 상주 daemon은 없다.
- **projection은 파일 내 data에서 재구축한다**: changelog는 `item_revisions`에서 유도하고
  gitignore된 `.flowmark/` journal에서 유도하지 않는다.

## T6에서 확정할 것

adapter별 field mapping, 일회성 command의 실행 시점, conflict 보고 형식, adapter 등록 schema.
