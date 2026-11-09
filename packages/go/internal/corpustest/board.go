package corpustest

// BoardDBS is the board catalog of the ops and command-line cases: a document list with a hierarchy, an order and a
// task marker, and a history table stored as a block of it; and a document table with neither.
const BoardDBS = `dbspec 1 board

table items {
  id varchar(32)
  parent varchar(32) null
  state varchar(1) default ' '
  title varchar(255)
  body text
  position i32
  owner varchar(32) null
  evidence text null
  primary key (id)
  index ix_items_parent (parent)
  index ix_items_owner (owner)
  foreign key fk_items_parent (parent) references items (id) on delete restrict on update restrict
  foreign key fk_items_owner (owner) references people (id) on delete restrict on update restrict
  settings {
    store document list
    title title
    body body
    order position
    markdown body
    markdown evidence
    task_marker state
  }
}

table item_history {
  id i64 identity
  item varchar(32)
  from_state varchar(16)
  to_state varchar(16)
  at datetime(6)
  evidence text null
  primary key (id)
  index ix_item_history_item (item)
  foreign key fk_history_item (item) references items (id) on delete restrict on update restrict
  settings {
    store block fk_history_item table
    markdown evidence
  }
}

table people {
  id varchar(32)
  name varchar(64)
  age i32 null
  primary key (id)
  settings {
    store document table
    title name
  }
}
`
