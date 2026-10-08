import type { JSX } from "solid-js";
import { render } from "solid-js/web";
import { Route, Router } from "@solidjs/router";
import "bulma/css/bulma.css";
import "./app.css";
import { AdminSocketProvider, RequireRole, SessionProvider } from "./session";
import Index from "./pages/Index";
import AdminRooms from "./pages/AdminRooms";
import AdminRoom from "./pages/AdminRoom";
import AdminLive from "./pages/AdminLive";
import UserRoom from "./pages/UserRoom";

function Root(props: { children?: JSX.Element }): JSX.Element {
  return (
    <SessionProvider>
      <AdminSocketProvider>{props.children}</AdminSocketProvider>
    </SessionProvider>
  );
}

const AdminListRoute = () => (
  <RequireRole role="admin">
    <AdminRooms />
  </RequireRole>
);

const AdminRoomRoute = () => (
  <RequireRole role="admin">
    <AdminRoom />
  </RequireRole>
);

const AdminLiveRoute = () => (
  <RequireRole role="admin">
    <AdminLive />
  </RequireRole>
);

const UserRoomRoute = () => (
  <RequireRole role="user">
    <UserRoom />
  </RequireRole>
);

const root = document.getElementById("root");
if (root) {
  render(
    () => (
      <Router root={Root}>
        <Route path="/" component={Index} />
        <Route path="/admin" component={AdminListRoute} />
        <Route path="/admin/rooms/:roomId" component={AdminRoomRoute} />
        <Route path="/admin/rooms/:roomId/live" component={AdminLiveRoute} />
        <Route path="/room/:roomId" component={UserRoomRoute} />
        <Route path="*" component={Index} />
      </Router>
    ),
    root,
  );
}
