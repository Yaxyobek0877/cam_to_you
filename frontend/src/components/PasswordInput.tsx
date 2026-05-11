import { useState, forwardRef } from "react";
import { Eye, EyeOff } from "lucide-react";

/**
 * PasswordInput — show/hide tugmasi bilan parol kiritish.
 * Camera password, YouTube stream key kabi maxfiy maydonlar uchun ishlatiladi.
 */
export interface PasswordInputProps extends Omit<React.InputHTMLAttributes<HTMLInputElement>, "type"> {
  /** Qo'shimcha className (input uchun) */
  className?: string;
  /** Boshlang'ich ko'rinish holati (default: yashirin) */
  defaultVisible?: boolean;
  /** Monospace shrift (stream key kabi uzun kodlar uchun) */
  monospace?: boolean;
}

export const PasswordInput = forwardRef<HTMLInputElement, PasswordInputProps>(
  ({ className = "input", defaultVisible = false, monospace = false, ...rest }, ref) => {
    const [visible, setVisible] = useState(defaultVisible);
    return (
      <div className="relative">
        <input
          ref={ref}
          {...rest}
          type={visible ? "text" : "password"}
          className={`${className} pr-10 ${monospace ? "font-mono text-xs" : ""}`}
          autoComplete="new-password"
        />
        <button
          type="button"
          onClick={() => setVisible((v) => !v)}
          className="absolute right-2 top-1/2 -translate-y-1/2 p-1.5 text-gray-400 hover:text-gray-100 rounded transition-colors"
          tabIndex={-1}
          title={visible ? "Yashirish" : "Ko'rsatish"}
        >
          {visible ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
        </button>
      </div>
    );
  },
);

PasswordInput.displayName = "PasswordInput";
