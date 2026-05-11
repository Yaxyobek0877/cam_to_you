import { NavLink, Outlet } from "react-router-dom";
import {
  LayoutDashboard,
  Camera,
  Radio,
  Settings,
  Minimize2,
  Power,
} from "lucide-react";
import { hideWindow, quitApp } from "../lib/api";
import { cn } from "../lib/utils";
import iconSvg from "../assets/icon.svg";

const navItems = [
  { to: "/dashboard", label: "Boshqaruv paneli", icon: LayoutDashboard },
  { to: "/cameras", label: "Kameralar", icon: Camera },
  { to: "/streams", label: "Streamlar", icon: Radio },
  { to: "/settings", label: "Sozlamalar", icon: Settings },
];

export function Layout() {
  return (
    <div className="flex h-screen w-screen bg-bg overflow-hidden">
      {/* Sidebar */}
      <aside className="w-60 flex-shrink-0 bg-bg-card border-r border-white/5 flex flex-col">
        {/* Logo / Header */}
        <div className="p-5 border-b border-white/5">
          <div className="flex items-center gap-2.5">
            <img src={iconSvg} alt="Cam2You" className="w-10 h-10 rounded-lg" />
            <div>
              <h1 className="text-base font-bold leading-tight">Cam2You</h1>
              <p className="text-xs text-gray-500 leading-tight">IP → YouTube</p>
            </div>
          </div>
        </div>

        {/* Nav links */}
        <nav className="flex-1 p-3 space-y-1">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) =>
                cn(
                  "flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm transition-colors",
                  isActive
                    ? "bg-accent text-white"
                    : "text-gray-300 hover:bg-bg-subtle hover:text-white",
                )
              }
            >
              <item.icon className="w-4 h-4" />
              {item.label}
            </NavLink>
          ))}
        </nav>

        {/* Bottom — tray & quit */}
        <div className="p-3 border-t border-white/5 space-y-1">
          <button
            onClick={() => hideWindow()}
            className="w-full flex items-center gap-3 px-3 py-2 rounded-lg text-sm text-gray-400 hover:bg-bg-subtle hover:text-white transition-colors"
            title="Tray'ga yashirish"
          >
            <Minimize2 className="w-4 h-4" />
            Tray'ga yashirish
          </button>
          <button
            onClick={() => {
              if (confirm("Dasturni butunlay yopmoqchimisiz? Barcha streamlar to'xtatiladi.")) {
                quitApp();
              }
            }}
            className="w-full flex items-center gap-3 px-3 py-2 rounded-lg text-sm text-gray-400 hover:bg-danger/20 hover:text-danger transition-colors"
            title="Dasturni butunlay yopish"
          >
            <Power className="w-4 h-4" />
            Chiqish
          </button>
        </div>
      </aside>

      {/* Asosiy kontent */}
      <main className="flex-1 overflow-auto">
        <Outlet />
      </main>
    </div>
  );
}
