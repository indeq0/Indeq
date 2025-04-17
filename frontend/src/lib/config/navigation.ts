import { Routes } from './sidebar-routes';
import { SearchIcon, PlusSquareIcon, SettingsIcon, ClockIcon} from 'svelte-feather-icons';

export const navigation = {
  main: [
    {
      label: "History", 
      url: "",
      icon: ClockIcon,
    },
  ],
  secondary: [
    {
      label: "Settings",
      url: Routes.profileSettings,
      icon: SettingsIcon,
    }
  ]
}; 