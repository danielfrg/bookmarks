<script lang="ts">
  import { Switch, SwitchThumb } from "@ui-sdk/svelte/switch";

  interface Props {
    label: string;
    checked: boolean;
    onchange: (checked: boolean) => void;
    disabled?: boolean;
  }
  let { label, checked, onchange, disabled = false }: Props = $props();
</script>

<label data-slot="label" class="switch-field">
  {label}
  <Switch {checked} {disabled} onCheckedChange={(next) => onchange(next)}>
    <SwitchThumb />
  </Switch>
</label>
