<article class="post">
<header class="board-head">
<h1>{= post.title}</h1>
<a href="/board" class="button secondary">목록</a>
</header>
<p class="post-meta">{= post.author} · {= date(post.created_at, 'Y-m-d H:i')}</p>
<div id="reader">{# reader}</div>
</article>
