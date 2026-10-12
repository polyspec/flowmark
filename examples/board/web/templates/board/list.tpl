<section class="board">
<header class="board-head">
<h1>게시판</h1>
<a href="/board/create" class="button">글쓰기</a>
</header>
{?# notice}<div id="notice">{# notice}</div>{/}
<div id="rows">{# rows}</div>
<nav class="pager">
{@ item = pager}
{? item.gap}
<span class="gap">…</span>
{:}
<a href="/board?sort={= sort}&page={= item.n}"{? item.n == page} class="current"{/}>{= item.n}</a>
{/}
{/}
</nav>
</section>
