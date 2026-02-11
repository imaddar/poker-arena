use serde::{Deserialize, Serialize};
use std::collections::BTreeSet;
use thiserror::Error;
use uuid::Uuid;

pub const DEFAULT_MAX_SEATS: u8 = 6;
pub const DEFAULT_MIN_PLAYERS_TO_START: u8 = 2;
pub const DEFAULT_STARTING_STACK: u32 = 10_000;
pub const DEFAULT_SMALL_BLIND: u32 = 50;
pub const DEFAULT_BIG_BLIND: u32 = 100;
pub const DEFAULT_ACTION_TIMEOUT_MS: u64 = 2_000;

#[derive(Debug, Error)]
pub enum DomainError {
    #[error("rank must be in range 2..=14, got {0}")]
    InvalidRank(u8),
    #[error("seat number must be in range 1..={max}, got {actual}")]
    InvalidSeatNo { max: u8, actual: u8 },
    #[error("action amount is required for {0:?}")]
    MissingActionAmount(ActionKind),
    #[error("action amount is not allowed for {0:?}")]
    UnexpectedActionAmount(ActionKind),
    #[error("table max_seats must be in range 2..=6, got {0}")]
    InvalidMaxSeats(u8),
    #[error("min_players_to_start must be at least 2 and <= max_seats")]
    InvalidMinPlayersToStart,
    #[error("big blind must be greater than or equal to small blind")]
    InvalidBlindStructure,
    #[error("hand must start with at least {minimum} active seats, got {actual}")]
    NotEnoughActiveSeats { minimum: u8, actual: usize },
    #[error("hand cannot exceed max seats ({max}), got {actual}")]
    TooManySeats { max: u8, actual: usize },
    #[error("duplicate seat numbers are not allowed")]
    DuplicateSeat,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum Suit {
    Clubs,
    Diamonds,
    Hearts,
    Spades,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct Rank(u8);

impl Rank {
    pub fn new(value: u8) -> Result<Self, DomainError> {
        if (2..=14).contains(&value) {
            Ok(Self(value))
        } else {
            Err(DomainError::InvalidRank(value))
        }
    }

    pub fn value(self) -> u8 {
        self.0
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct Card {
    pub rank: Rank,
    pub suit: Suit,
}

impl Card {
    pub fn new(rank: Rank, suit: Suit) -> Self {
        Self { rank, suit }
    }
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Deck {
    cards: Vec<Card>,
}

impl Deck {
    pub fn standard_52() -> Self {
        let mut cards = Vec::with_capacity(52);

        for suit in [Suit::Clubs, Suit::Diamonds, Suit::Hearts, Suit::Spades] {
            for rank in 2..=14 {
                let rank = Rank::new(rank).expect("standard deck ranks are always valid");
                cards.push(Card::new(rank, suit));
            }
        }

        Self { cards }
    }

    pub fn cards(&self) -> &[Card] {
        &self.cards
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum Street {
    Preflop,
    Flop,
    Turn,
    River,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ActionKind {
    Fold,
    Check,
    Call,
    Bet,
    Raise,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub struct Action {
    pub kind: ActionKind,
    pub amount: Option<u32>,
}

impl Action {
    pub fn new(kind: ActionKind, amount: Option<u32>) -> Result<Self, DomainError> {
        let needs_amount = matches!(kind, ActionKind::Bet | ActionKind::Raise);

        if needs_amount && amount.is_none() {
            return Err(DomainError::MissingActionAmount(kind));
        }

        if !needs_amount && amount.is_some() {
            return Err(DomainError::UnexpectedActionAmount(kind));
        }

        Ok(Self { kind, amount })
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash, Serialize, Deserialize)]
pub struct SeatNo(u8);

impl SeatNo {
    pub fn new(value: u8, max_seats: u8) -> Result<Self, DomainError> {
        if value == 0 || value > max_seats {
            Err(DomainError::InvalidSeatNo {
                max: max_seats,
                actual: value,
            })
        } else {
            Ok(Self(value))
        }
    }

    pub fn value(self) -> u8 {
        self.0
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum SeatStatus {
    Active,
    SittingOut,
    Busted,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct SeatState {
    pub seat_no: SeatNo,
    pub stack: u32,
    pub committed_in_round: u32,
    pub status: SeatStatus,
}

impl SeatState {
    pub fn new(seat_no: SeatNo, stack: u32) -> Self {
        Self {
            seat_no,
            stack,
            committed_in_round: 0,
            status: SeatStatus::Active,
        }
    }

    pub fn is_active(&self) -> bool {
        self.status == SeatStatus::Active
    }
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct TableConfig {
    pub max_seats: u8,
    pub min_players_to_start: u8,
    pub starting_stack: u32,
    pub small_blind: u32,
    pub big_blind: u32,
    pub action_timeout_ms: u64,
}

impl TableConfig {
    pub fn default_v0() -> Self {
        Self {
            max_seats: DEFAULT_MAX_SEATS,
            min_players_to_start: DEFAULT_MIN_PLAYERS_TO_START,
            starting_stack: DEFAULT_STARTING_STACK,
            small_blind: DEFAULT_SMALL_BLIND,
            big_blind: DEFAULT_BIG_BLIND,
            action_timeout_ms: DEFAULT_ACTION_TIMEOUT_MS,
        }
    }

    pub fn validate(&self) -> Result<(), DomainError> {
        if !(2..=DEFAULT_MAX_SEATS).contains(&self.max_seats) {
            return Err(DomainError::InvalidMaxSeats(self.max_seats));
        }

        if self.min_players_to_start < 2 || self.min_players_to_start > self.max_seats {
            return Err(DomainError::InvalidMinPlayersToStart);
        }

        if self.big_blind < self.small_blind {
            return Err(DomainError::InvalidBlindStructure);
        }

        Ok(())
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum HandPhase {
    Dealing,
    Betting(Street),
    Showdown,
    Complete,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct HandState {
    pub hand_id: Uuid,
    pub table_id: Uuid,
    pub hand_no: u64,
    pub button_seat: SeatNo,
    pub acting_seat: SeatNo,
    pub phase: HandPhase,
    pub pot: u32,
    pub board: Vec<Card>,
    pub seats: Vec<SeatState>,
}

impl HandState {
    pub fn new(
        table_id: Uuid,
        hand_no: u64,
        button_seat: SeatNo,
        acting_seat: SeatNo,
        seats: Vec<SeatState>,
        config: &TableConfig,
    ) -> Result<Self, DomainError> {
        config.validate()?;

        let active_count = seats.iter().filter(|seat| seat.is_active()).count();
        if active_count < config.min_players_to_start as usize {
            return Err(DomainError::NotEnoughActiveSeats {
                minimum: config.min_players_to_start,
                actual: active_count,
            });
        }

        if seats.len() > config.max_seats as usize {
            return Err(DomainError::TooManySeats {
                max: config.max_seats,
                actual: seats.len(),
            });
        }

        let unique_count = seats
            .iter()
            .map(|seat| seat.seat_no)
            .collect::<BTreeSet<_>>()
            .len();
        if unique_count != seats.len() {
            return Err(DomainError::DuplicateSeat);
        }

        Ok(Self {
            hand_id: Uuid::new_v4(),
            table_id,
            hand_no,
            button_seat,
            acting_seat,
            phase: HandPhase::Dealing,
            pot: 0,
            board: Vec::with_capacity(5),
            seats,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn action_validation_requires_amount_for_raise() {
        let err = Action::new(ActionKind::Raise, None).expect_err("raise must require an amount");
        assert!(matches!(err, DomainError::MissingActionAmount(_)));
    }

    #[test]
    fn hand_state_rejects_duplicate_seats() {
        let cfg = TableConfig::default_v0();
        let seat_no = SeatNo::new(1, cfg.max_seats).expect("seat is valid");
        let duplicate = vec![
            SeatState::new(seat_no, cfg.starting_stack),
            SeatState::new(seat_no, cfg.starting_stack),
        ];

        let err = HandState::new(
            Uuid::new_v4(),
            1,
            seat_no,
            seat_no,
            duplicate,
            &cfg,
        )
        .expect_err("duplicate seats must fail");

        assert!(matches!(err, DomainError::DuplicateSeat));
    }
}
