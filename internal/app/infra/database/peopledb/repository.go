package peopledb

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2/log"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/leorcvargas/rinha-2023-q3/internal/app/domain/people"
	"github.com/redis/rueidis"
)

type PersonRepository struct {
	db       *pgxpool.Pool
	cache    *PeopleDbCache
	jobQueue JobQueue
}

func (p *PersonRepository) Create(person *people.Person) error {
	p.cache.Set(person.ID, person)

	p.jobQueue <- Job{Payload: person}

	return nil
}

func (p *PersonRepository) FindByID(id string) (*people.Person, error) {
	cachedPerson, err := p.cache.Get(id)

	if err != nil && !rueidis.IsRedisNil(err) {
		log.Errorf("Error getting person from cache: %v", err)
		return nil, err
	}

	if cachedPerson != nil {
		return cachedPerson, nil
	}

	var person people.Person
	var strStack string
	var birthdate time.Time

	err = p.db.QueryRow(
		context.Background(),
		SelectPersonByIDQuery,
		id,
	).Scan(
		&person.ID,
		&person.Nickname,
		&person.Name,
		&birthdate,
		&strStack,
	)
	person.Stack = strings.Split(strStack, ",")
	person.Birthdate = birthdate.Format("2006-01-02")

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, people.ErrPersonNotFound
		}

		log.Errorf("Error querying person: %v", err)

		return nil, err
	}

	return &person, nil
}

func (p *PersonRepository) Search(term string) ([]people.Person, error) {
	rows, err := p.db.Query(
		context.Background(),
		SearchPeopleTrgmQuery,
		term,
	)
	if err != nil {
		log.Errorf("Error executing trigram search: %v", err)
		return nil, err
	}

	return p.mapSearchResult(rows)
}

func (p *PersonRepository) CountAll() (int64, error) {
	var total int64

	err := p.db.
		QueryRow(
			context.Background(),
			CountPeopleQuery,
		).
		Scan(&total)

	if err != nil {
		log.Errorf("Error counting people: %v", err)
		return 0, err
	}

	return total, nil
}

func (p *PersonRepository) CheckNicknameExists(nickname string) (bool, error) {
	nicknameTaken, err := p.cache.GetNickname(nickname)
	if err != nil {
		return false, err
	}

	if nicknameTaken {
		return true, nil
	}

	return false, nil
}

func (p *PersonRepository) mapSearchResult(rows pgx.Rows) ([]people.Person, error) {
	result := make([]people.Person, 0)
	for rows.Next() {
		var person people.Person
		var strStack string
		var birthdate time.Time

		err := rows.Scan(
			&person.ID,
			&person.Nickname,
			&person.Name,
			&birthdate,
			&strStack,
		)
		if err != nil {
			log.Errorf("Error scanning row: %v", err)
			return nil, err
		}

		person.Stack = strings.Split(strStack, ",")
		person.Birthdate = birthdate.Format("2006-01-02")

		result = append(result, person)
	}

	return result, nil
}

func NewPersonRepository(db *pgxpool.Pool, cache *PeopleDbCache, jobQueue JobQueue) people.Repository {
	return &PersonRepository{
		db:       db,
		cache:    cache,
		jobQueue: jobQueue,
	}
}
