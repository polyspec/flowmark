<nav class="left">
<h2>메뉴</h2>
<ul>
<li><a href="/"{? path == '/'} class="active"{/}>홈</a></li>
<li><a href="/board"{? starts_with(path, '/board')} class="active"{/}>게시판 <span class="badge">{= count}</span></a></li>
</ul>
</nav>
