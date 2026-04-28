async function loadDashboard() {
  const response = await fetch("data/project-data.json");
  const data = await response.json();

  document.getElementById("project-name").textContent = data.name;
  document.getElementById("project-summary").textContent = data.summary;
  document.getElementById("release-target").textContent = data.releaseTarget;
  document.getElementById("primary-stack").textContent = data.primaryStack;
  document.getElementById("last-cleanup").textContent = data.lastCleanup;

  renderToolCards(document.getElementById("tools"), data.tools);
  renderList(document.getElementById("architecture"), data.architecture);
  renderList(document.getElementById("docs"), data.docs);
  renderList(document.getElementById("dependencies"), data.dependencies);
  renderList(document.getElementById("open-issues"), data.openIssues);
  renderList(document.getElementById("limitations"), data.limitations);
  renderList(document.getElementById("testing"), data.testing);
  renderList(document.getElementById("recent-changes"), data.recentChanges);
}

function renderToolCards(root, items) {
  root.innerHTML = "";
  items.forEach((item) => {
    const card = document.createElement("article");
    card.className = "tool-card";

    const title = document.createElement("h3");
    title.textContent = item.name;
    card.appendChild(title);

    const body = document.createElement("p");
    body.textContent = item.description;
    card.appendChild(body);

    root.appendChild(card);
  });
}

function renderList(root, items) {
  root.innerHTML = "";
  items.forEach((item) => {
    const li = document.createElement("li");
    li.textContent = item;
    root.appendChild(li);
  });
}

loadDashboard().catch((error) => {
  document.body.innerHTML = `<main class="dashboard"><section class="hero"><p class="eyebrow">Project Dashboard</p><h1>Dashboard load failed</h1><p class="summary">${error.message}</p></section></main>`;
});
