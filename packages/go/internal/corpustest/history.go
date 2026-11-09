package corpustest

// TicketHistoryTable is the dbspec of a table ticket_history stored as a document table: the history of a table
// tickets that the application keeps, an ordinary table, since the engine holds no state machine (spec 50 State
// columns).
const TicketHistoryTable = `table ticket_history {
  id i64 identity
  ticket varchar(16)
  from_state varchar(16)
  to_state varchar(16)
  at datetime(6)
  primary key (id)
  index ix_ticket_history_ticket (ticket)
  foreign key fk_ticket_history_ticket (ticket) references tickets (id) on delete restrict on update restrict
  settings {
    store document table
  }
}
`
