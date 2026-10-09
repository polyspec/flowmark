<!-- doc-id: spec-ops -->
<!-- source-sha256: 911da057c4547e98ce846d34d5e066ff198d55df34a829d8100262bfc5c101b9 -->
# 운영

[English](40-ops.md)

이 명세는 운영 의미론을 확정한다: corpus 열기, transaction protocol, lock, revision token, 비용
모델. commit protocol의 순서는 규범적이다.

## Corpus 열기

- 열기는 **복구를 먼저 실행한다**: prepare record가 있는 stage는 완료하고, 없는 stage는 폐기하며,
  대상의 현재 hash가 기록된 어느 쪽과도 다르면 — crash 사이에 사람 편집이 들어왔다는 뜻 — 거부하고
  보고한다, 결코 조용히 덮어쓰지 않는다.
- walk가 corpus view를 만든다: schema가 묶은 folder, parse된 문서, 유도된 index, file content
  hash로 key가 지어진 parsed-row cache를 변하지 않은 파일에 재사용.

## Lock

- **하나의 portable lock, 작업 단위.** 모든 작업 — 읽기와 쓰기, CLI·MCP·sidecar — 는 같은 lock
  file을 holder 신원·stale 판정과 함께 자기 기간 동안 획득하고, timeout은 holder를 지목한다. 장기
  lock도 없고 reader/writer 구분도 없고 daemon도 없다: face는 host의 생명주기 안에서 실행된다.
- commit 충돌 — 읽은 뒤 대상이 바뀜 — 은 file과 line과 함께 실패하고 정의된 **재독기-재시도-1회**
  계약을 따른다. silent merge는 결코 없다.
- Multi-platform 근거: flock 없음, OS별 durability 없음; lock은 holder 신원을 가진 lock file(kit
  holder-lock 선례)이고 crash 안전성은 아래 journal replay에서 온다.

## Commit protocol

1. 모든 post-image 파일을 `.flowmark/tx/<id>/`에 content-addressed로 쓰고 fsync한다.
2. 모든 대상의 pre-image를 stage로 복사하고 fsync한다.
3. 모든 rename과 pre/post hash를 적은 **prepare record**를 쓰고 fsync한다.
4. runtime의 atomic rename으로 rename을 수행한다.
5. commit record를 쓰고 fsync한다.
6. stage를 정리한다.

검증은 commit 시점에 **lock 아래에서 corpus를 새로 읽어** 수행한다 — BEGIN snapshot도, 예측된
touch file 집합도 아니다 (참조 탐색이 그 집합을 미리 계산 불가능하게 만든다). 정전 이후의
durability는 이 journal과 열기 시 복구에 의존하며, OS별 directory fsync 의미론에 의존하지 않는다.

## Corpus revision

- `.flowmark/`의 단조 증가 counter이며 모든 commit이 올린다. 다중 호출 read session — paginate —
  은 token을 가지고 다니며 불일치 시 `CORPUS_CHANGED`로 실패하거나 keyset pagination으로
  강등한다. `orderByRandom`은 revision별 seed를 쓴다.

## 비용 모델

- per-command p95 latency 목표를 여기에 선언한다 ([open questions](110-open-questions.ko.md)의
  corpus envelope 기준 수백 ms).
- parsed-row cache가 변하지 않은 파일의 재parse를 피한다; 기본 per-transaction 검증은
  post-image의 FK/UK closure이고, 전체 `flowmark validate`는 on demand와 schema 변경 시에
  실행한다.
- 장기 face(sidecar)는 읽기를 위해 corpus를 watch하고 host에 변경 event를 push할 수 있다; write는
  여전히 transaction protocol로만 간다.

## 실패 어휘

`LOCK_TIMEOUT`은 holder를 지목한다; `CORPUS_CONFLICT`는 file과 line을; `CORPUS_CHANGED`는
revision을; 복구 거부는 대상과 기록된 양쪽 hash를 지목한다. 모든 실패 message는 원인과 해결법을
명시한다.
