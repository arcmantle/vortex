console.log("Mock Vite app running");

document.querySelector("#app").insertAdjacentHTML(
  "beforeend",
  `<p>Started at ${new Date().toLocaleTimeString()}</p>`
);
