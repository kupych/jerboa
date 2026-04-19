import "./app.css";
import App from "./App.svelte";
import { mount } from "svelte";
import { initAnalytics } from "./lib/analytics";

const app = mount(App, { target: document.getElementById("app")! });

initAnalytics();

export default app;
