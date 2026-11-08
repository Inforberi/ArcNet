import {PageHeading} from '../shared/ui';
import UpdateSettings from '../features/settings/UpdateSettings';
export default function Versions(){return <><PageHeading title="Версии ArcNet" description="Установленная версия, доступные обновления и результат установки"/><UpdateSettings active/></>}
