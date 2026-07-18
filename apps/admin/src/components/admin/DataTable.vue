<template>
  <div class="admin-data-table">
    <table>
      <thead>
        <tr v-for="headerGroup in table.getHeaderGroups()" :key="headerGroup.id">
          <th v-for="header in headerGroup.headers" :key="header.id">
            <FlexRender v-if="!header.isPlaceholder" :render="header.column.columnDef.header" :props="header.getContext()" />
          </th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in table.getRowModel().rows" :key="row.id">
          <td v-for="cell in row.getVisibleCells()" :key="cell.id">
            <FlexRender :render="cell.column.columnDef.cell" :props="cell.getContext()" />
          </td>
        </tr>
        <tr v-if="!table.getRowModel().rows.length"><td :colspan="columns.length">暂无数据</td></tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts" generic="TData">
import { FlexRender, getCoreRowModel, useVueTable, type ColumnDef } from "@tanstack/vue-table";

const props = defineProps<{ data: TData[]; columns: ColumnDef<TData, unknown>[] }>();
const table = useVueTable({
  get data() { return props.data; },
  columns: props.columns,
  getCoreRowModel: getCoreRowModel(),
});
</script>
