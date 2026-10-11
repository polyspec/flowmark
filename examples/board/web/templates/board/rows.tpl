<div class="board-tools">
<span>정렬</span>
<button type="button" class="chip{? sort == ''} current{/}" hy-set='sort=""'>최신순</button>
<button type="button" class="chip{? sort == 'title'} current{/}" hy-set='sort="title"'>제목순</button>
<button type="button" class="chip{? sort == 'author'} current{/}" hy-set='sort="author"'>작성자순</button>
<button type="button" class="chip{? compact} current{/}" hy-set="compact={? compact}false{:}true{/}">좁게 보기</button>
</div>
<table class="board-table{? compact} compact{/}">
<thead>
<tr><th>번호</th><th>제목</th><th>작성자</th><th>작성일</th></tr>
</thead>
<tbody>
{:shown = posts}
{? sort == 'title'}
{:shown = sort(posts, 'title')}
{:? sort == 'author'}
{:shown = sort(posts, 'author')}
{/}
{@ post = shown}
<tr id="post-{= post.id}"{? post.id == highlight} class="new"{/}>
<td>{= post.id}</td>
<td><a href="/board/{= post.id}">{= post.title}</a></td>
<td>{= post.author}</td>
<td>{= date(post.created_at, 'Y-m-d H:i')}</td>
</tr>
{:}
<tr><td colspan="4" class="empty">게시글이 없습니다.</td></tr>
{/}
</tbody>
</table>
