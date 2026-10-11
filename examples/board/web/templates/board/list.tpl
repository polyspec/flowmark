<section class="board">
<header class="board-head">
<h1>게시판</h1>
<a href="/board/create" class="button">글쓰기</a>
</header>
{?# notice}<div id="notice">{# notice}</div>{/}
<div id="rows">{# rows}</div>
<nav class="pager">
{@ n = range(1, pages)}
<a href="/board?page={= n}"{? n == page} class="current"{/}>{= n}</a>
{/}
</nav>
</section>
